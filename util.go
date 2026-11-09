package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/spf13/afero"
)

// humanizeBytes formats a byte count using binary units, e.g. 1536 -> "1.5 KiB".
func humanizeBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func loadTemplates(templateFS fs.FS) (*template.Template, error) {
	tplFuncMap := template.FuncMap{
		"humanize": humanizeBytes,
	}

	tmpl := template.New("")
	tmpl.Funcs(tplFuncMap)
	tmpl, err := tmpl.ParseFS(templateFS, "**/*")
	if err != nil {
		return nil, err
	}

	return tmpl, nil
}

func IOFSFromS3URL(ctx context.Context, client *s3.Client, url *url.URL) (fs.FS, error) {
	if url.Scheme != "s3" {
		return nil, errors.New("requires s3 URL")
	}

	return newS3ReadFS(ctx, client, url.Host, strings.TrimPrefix(url.Path, "/")), nil
}

func FSFromBucketURL(ctx context.Context, client *s3.Client, bucketURL *url.URL) (fs.FS, error) {
	if bucketURL == nil {
		return nil, errors.New("bucket URL is required")
	}

	s3Fs, err := IOFSFromS3URL(ctx, client, bucketURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load FS from %v: %w", bucketURL.Redacted(), err)
	}

	return s3Fs, nil
}

func FSFromS3URLOrDefault(ctx context.Context, client *s3.Client, s3URL *url.URL, defaultFS fs.FS) (fs.FS, error) {
	if s3URL != nil {
		return FSFromBucketURL(ctx, client, s3URL)
	}

	return defaultFS, nil
}

func CopyFile(destFS afero.Fs, srcFS fs.FS) fs.WalkDirFunc {
	return func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return destFS.MkdirAll(d.Name(), 0755)
		}

		srcFile, err := srcFS.Open(p)
		if err != nil {
			return fmt.Errorf("failed to open source path: %w", err)
		}
		defer func() { _ = srcFile.Close() }()

		destFile, err := destFS.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open destination path: %w", err)
		}

		if _, err = io.Copy(destFile, srcFile); err != nil {
			_ = destFile.Close()

			return fmt.Errorf("failed to copy: %w", err)
		}

		return destFile.Close()
	}
}

func CopyFilesFromSubPath(destFS afero.Fs, srcFS fs.FS, srcPath string) error {
	f := CopyFile(destFS, srcFS)

	return fs.WalkDir(srcFS, srcPath, f)
}

func NewLocalOutputFS(localOutputDirectory string) (afero.Fs, error) {
	o := afero.NewOsFs()
	mkdirErr := o.MkdirAll(localOutputDirectory, 0755)
	if mkdirErr != nil {
		return nil, fmt.Errorf("failed to mkdir: %w", mkdirErr)
	}

	return afero.NewBasePathFs(o, localOutputDirectory), nil
}

func NewS3OutputFS(ctx context.Context, client *s3.Client, bucketName string, prefix string, serverSideEncryption *string) afero.Fs {
	bucketKeyEnabled := true
	var sse s3types.ServerSideEncryption
	if serverSideEncryption != nil {
		if *serverSideEncryption == "" {
			bucketKeyEnabled = false
		} else {
			sse = s3types.ServerSideEncryption(*serverSideEncryption)
		}
	}

	cacheControl := fmt.Sprintf("max-age=%d", (time.Minute/time.Second)*5)

	props := &S3FileProps{
		CacheControl:         &cacheControl,
		ServerSideEncryption: sse,
		BucketKeyEnabled:     &bucketKeyEnabled,
	}

	var outputFS afero.Fs = &s3WriteFS{
		ctx:    ctx,
		client: client,
		bucket: bucketName,
		props:  props,
	}

	if prefix != "" {
		outputFS = afero.NewBasePathFs(outputFS, prefix)
	}

	return outputFS
}

// signedFS is implemented by file systems that can cheaply tell whether their contents have changed.
type signedFS interface {
	// signature returns a value that changes whenever any file in the file system changes.
	signature() (string, error)
}

// embeddedSignature stands in for the signature of the embedded assets, which cannot change while the
// process is running.
const embeddedSignature = "embedded"

var (
	templateCacheMu sync.Mutex
	templateCache   struct {
		signature string
		tmpl      *template.Template
	}

	staticCopyMu sync.Mutex
	staticCopy   struct {
		dest      string
		signature string
	}
)

// resetCaches forgets the cached templates and static copy, forcing the next run to rebuild both.
func resetCaches() {
	templateCacheMu.Lock()
	templateCache.signature, templateCache.tmpl = "", nil
	templateCacheMu.Unlock()

	staticCopyMu.Lock()
	staticCopy.dest, staticCopy.signature = "", ""
	staticCopyMu.Unlock()
}

func sourceSignature(fsys fs.FS) (string, bool, error) {
	signed, ok := fsys.(signedFS)
	if !ok {
		return embeddedSignature, false, nil
	}

	sig, err := signed.signature()

	return sig, true, err
}

// LoadTemplates parses the templates from templateBucketURL, or the embedded ones when it is nil. Templates
// read from S3 are reused across invocations of a warm Lambda for as long as their ETags are unchanged.
func LoadTemplates(ctx context.Context, client *s3.Client, templateBucketURL *url.URL) (*template.Template, error) {
	tmplFS, err := FSFromS3URLOrDefault(ctx, client, templateBucketURL, defaultTemplateFS)
	if err != nil {
		return nil, err
	}

	return loadCachedTemplates(tmplFS)
}

func loadCachedTemplates(tmplFS fs.FS) (*template.Template, error) {
	sig, signed, err := sourceSignature(tmplFS)
	if err != nil {
		return nil, fmt.Errorf("failed to check templates for changes: %w", err)
	}
	if !signed {
		return loadTemplates(tmplFS)
	}

	templateCacheMu.Lock()
	defer templateCacheMu.Unlock()

	if templateCache.tmpl != nil && templateCache.signature == sig {
		return templateCache.tmpl, nil
	}

	tmpl, err := loadTemplates(tmplFS)
	if err != nil {
		return nil, err
	}
	templateCache.signature, templateCache.tmpl = sig, tmpl

	return tmpl, nil
}

// CopyStaticFiles copies the static assets from staticBucketURL, or the embedded ones when it is nil, into
// destFS. destKey names the destination (e.g. bucket and prefix); a copy already made to the same
// destKey from unchanged assets is not repeated.
func CopyStaticFiles(ctx context.Context, client *s3.Client, destFS afero.Fs, destKey string, staticBucketURL *url.URL) error {
	staticFS, err := FSFromS3URLOrDefault(ctx, client, staticBucketURL, defaultStaticFS)
	if err != nil {
		return fmt.Errorf("failed to load static assets from bucket %v: %w", staticBucketURL, err)
	}

	return copyStaticOnce(destFS, destKey, staticFS)
}

func copyStaticOnce(destFS afero.Fs, destKey string, staticFS fs.FS) error {
	sig, _, err := sourceSignature(staticFS)
	if err != nil {
		return fmt.Errorf("failed to check static assets for changes: %w", err)
	}

	staticCopyMu.Lock()
	defer staticCopyMu.Unlock()

	if staticCopy.dest == destKey && staticCopy.signature == sig {
		return nil
	}

	if err := CopyFilesFromSubPath(destFS, staticFS, "static"); err != nil {
		return fmt.Errorf("failed to copy static files %w", err)
	}
	staticCopy.dest, staticCopy.signature = destKey, sig

	return nil
}

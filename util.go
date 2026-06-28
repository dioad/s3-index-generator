package main

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/spf13/afero"
)

func loadTemplates(templateFS fs.FS) (*template.Template, error) {
	tplFuncMap := make(template.FuncMap)

	tmpl := template.New("")
	tmpl.Funcs(tplFuncMap)
	tmpl, err := tmpl.ParseFS(templateFS, "**/*")
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}

func IOFSFromS3URL(client *s3.Client, url *url.URL) (fs.FS, error) {
	if url.Scheme != "s3" {
		return nil, errors.New("requires s3 URL")
	}
	return newS3ReadFS(client, url.Host, strings.TrimPrefix(url.Path, "/")), nil
}

func FSFromBucketURL(client *s3.Client, bucketURL *url.URL) (fs.FS, error) {
	if bucketURL != nil {
		s3Fs, err := IOFSFromS3URL(client, bucketURL)
		if err != nil {
			return nil, fmt.Errorf("failed to load FS from %v: %v", bucketURL.Redacted(), err)
		}
		return s3Fs, nil
	}
	return nil, nil
}

func FSFromS3URLOrDefault(client *s3.Client, s3URL *url.URL, defaultFS fs.FS) (fs.FS, error) {
	if s3URL != nil {
		return FSFromBucketURL(client, s3URL)
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
			return fmt.Errorf("failed to open source path: %v", err)
		}
		defer func() { _ = srcFile.Close() }()

		destFile, err := destFS.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open destination path: %v", err)
		}

		if _, err = io.Copy(destFile, srcFile); err != nil {
			_ = destFile.Close()
			return fmt.Errorf("failed to copy: %v", err)
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
		return nil, fmt.Errorf("failed to mkdir: %v", mkdirErr)
	}
	return afero.NewBasePathFs(o, localOutputDirectory), nil
}

func NewS3OutputFS(client *s3.Client, bucketName string, prefix string, serverSideEncryption *string) afero.Fs {
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
		client: client,
		bucket: bucketName,
		props:  props,
	}

	if prefix != "" {
		outputFS = afero.NewBasePathFs(outputFS, prefix)
	}

	return outputFS
}

func LoadTemplates(client *s3.Client, templateBucketURL *url.URL) (*template.Template, error) {
	tmplFS, err := FSFromS3URLOrDefault(client, templateBucketURL, defaultTemplateFS)
	if err != nil {
		return nil, err
	}

	tmpl, err := loadTemplates(tmplFS)
	if err != nil {
		return nil, err
	}

	return tmpl, err
}

func CopyStaticFiles(client *s3.Client, destFS afero.Fs, staticBucketURL *url.URL) error {
	staticFS, err := FSFromS3URLOrDefault(client, staticBucketURL, defaultStaticFS)
	if err != nil {
		return fmt.Errorf("failed to load static assets from bucket %v: %w", staticBucketURL, err)
	}

	err = CopyFilesFromSubPath(destFS, staticFS, "static")
	if err != nil {
		return fmt.Errorf("failed to copy static files %w", err)
	}

	return nil
}

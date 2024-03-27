package main

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	aferos3 "github.com/fclairamb/afero-s3"
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

func IOFSFromS3URL(sess *session.Session, url *url.URL) (fs.FS, error) {
	if url.Scheme != "s3" {
		return nil, errors.New("requires s3 URL")
	}

	aferoFS := aferos3.NewFs(url.Host, sess)
	subPathFS := afero.NewBasePathFs(aferoFS, url.Path)
	ioFS := afero.NewIOFS(subPathFS)

	return ioFS, nil
}

func FSFromBucketURL(sess *session.Session, bucketURL *url.URL) (fs.FS, error) {
	if bucketURL != nil {
		s3Fs, err := IOFSFromS3URL(sess, bucketURL)
		if err != nil {
			return nil, fmt.Errorf("failed to load FS from %v: %v", bucketURL.Redacted(), err)
		}
		return s3Fs, nil
	}
	return nil, nil
}

func FSFromS3URLOrDefault(sess *session.Session, s3URL *url.URL, defaultFS fs.FS) (fs.FS, error) {
	var f fs.FS
	var err error
	f = defaultFS
	if s3URL != nil {
		f, err = FSFromBucketURL(sess, s3URL)
		if err != nil {
			return nil, err

		}
	}
	return f, nil
}

func CopyFile(destFS afero.Fs, srcFS fs.FS) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			mkdirErr := destFS.MkdirAll(d.Name(), 0755)
			if mkdirErr != nil {
				return fmt.Errorf("failed to mkdir: %v", mkdirErr)
			}
			return nil
		}

		if !d.IsDir() {
			srcFile, err := srcFS.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open source path: %v", err)
			}
			destFile, err := destFS.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return fmt.Errorf("failed to open destination path: %v", err)
			}

			_, err = io.Copy(destFile, srcFile)
			if err != nil {
				return fmt.Errorf("failed to copy: %v", err)
			}

			defer destFile.Close()
			defer srcFile.Close()
		}
		return nil
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

func NewS3OutputFS(sess *session.Session, bucketName string, prefix string, serverSideEncryption *string) afero.Fs {
	bucketKeyEnabled := true
	if serverSideEncryption != nil && *serverSideEncryption == "" {
		serverSideEncryption = nil
		bucketKeyEnabled = false
	}

	cacheControl := fmt.Sprintf("max-age=%d", (time.Minute/time.Second)*5)

	fileProps := &aferos3.UploadedFileProperties{
		CacheControl:         &cacheControl,
		ServerSideEncryption: serverSideEncryption,
		BucketKeyEnabled:     &bucketKeyEnabled,
	}

	sp := aferos3.NewFs(bucketName, sess)
	sp.FileProps = fileProps

	var s afero.Fs
	if prefix != "" {
		s = afero.NewBasePathFs(sp, prefix)
	} else {
		s = sp
	}

	return s
}

func LoadTemplates(sess *session.Session, templateBucketURL *url.URL) (*template.Template, error) {
	tmplFS, err := FSFromS3URLOrDefault(sess, templateBucketURL, defaultTemplateFS)
	if err != nil {
		return nil, err
	}

	tmpl, err := loadTemplates(tmplFS)
	if err != nil {
		return nil, err
	}

	return tmpl, err
}

func CopyStaticFiles(sess *session.Session, destFS afero.Fs, staticBucketURL *url.URL) error {
	staticFS, err := FSFromS3URLOrDefault(sess, staticBucketURL, defaultStaticFS)
	if err != nil {
		return fmt.Errorf("failed to load static assets from bucket %v: %w", staticBucketURL, err)
	}

	// copy static
	err = CopyFilesFromSubPath(destFS, staticFS, "static")
	if err != nil {
		return fmt.Errorf("failed to copy static files %w", err)
	}

	return nil
}

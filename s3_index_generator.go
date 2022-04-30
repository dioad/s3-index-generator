package main

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-xray-sdk-go/xray"
	aferos3 "github.com/fclairamb/afero-s3"

	//"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/spf13/afero"
)

//go:embed templates
var defaultTemplateFS embed.FS

//go:embed static
var defaultStaticFS embed.FS

func ObjectBaseName(objKey string) string {
	parts := strings.Split(objKey, "/")
	return parts[len(parts)-1]
}

func loadTemplates(templateFS fs.FS) (*template.Template, error) {
	tplFuncMap := make(template.FuncMap)
	tplFuncMap["ObjectBaseName"] = ObjectBaseName

	tmpl := template.New("")
	tmpl.Funcs(tplFuncMap)
	tmpl, err := tmpl.ParseFS(templateFS, "**/*")
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}

func fetchBucketObjectTree(s3Client *s3.S3, objectBucketName string, objectPrefix string) (*ObjectTree, error) {
	objInput := s3.ListObjectsInput{
		Bucket: &objectBucketName,
		Prefix: &objectPrefix,
	}
	objects, err := s3Client.ListObjects(&objInput)
	if err != nil {
		return nil, err
	}

	t := NewRootObjectTree()
	t.PrefixToStrip = objectPrefix
	t.Exclusions = Exclusions{
		ExcludeKey("favicon.ico"),
		ExcludeKey("index.html"),
		ExcludePrefix("."),
		ExcludeSuffix("/"),
		ExcludeSuffix("/index.html"),
	}

	AddObjectsToTree(t, objects.Contents)

	return t, nil
}

type Page struct {
	Nonce      string
	ObjectTree *ObjectTree
}

func nonce() string {
	nonce := make([]byte, 6)
	_, err := rand.Read(nonce)
	if err != nil {
		log.Fatalf("failed to generate nonce: %v", err)
	}
	return base64.StdEncoding.EncodeToString(nonce)
}

func renderObjectTreeAsSinglePage(objectTree *ObjectTree, tmpl *template.Template, templateName string, destFS afero.Fs) error {
	f, err := destFS.OpenFile(IndexFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	p := Page{
		Nonce:      nonce(),
		ObjectTree: objectTree,
	}

	return tmpl.ExecuteTemplate(f, templateName, p)
}

func renderObjectTreeAsMultiPage(objectTree *ObjectTree, tmpl *template.Template, templateName string, destFS afero.Fs) error {
	err := renderObjectTreeAsSinglePage(objectTree, tmpl, templateName, destFS)
	if err != nil {
		return err
	}

	for _, v := range objectTree.Children {
		err := destFS.MkdirAll(v.DirName, 0755)
		if err != nil {
			return err
		}

		subFS := afero.NewBasePathFs(destFS, v.DirName)
		err = renderObjectTreeAsMultiPage(v, tmpl, templateName, subFS)

		if err != nil {
			return err
		}
	}

	return nil
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

func getFSFromBucketURL(bucketURL *url.URL, sess *session.Session) (fs.FS, error) {
	if bucketURL != nil {
		s3Fs, err := IOFSFromS3URL(sess, bucketURL)
		if err != nil {
			return nil, fmt.Errorf("failed to load FS from %v: %v", bucketURL.Redacted(), err)
		}
		return s3Fs, nil
	}
	return nil, nil
}

func getFSFromS3URLOrDefault(s3URL *url.URL, sess *session.Session, defaultFS fs.FS) (fs.FS, error) {
	var f fs.FS
	var err error
	f = defaultFS
	if s3URL != nil {
		f, err = getFSFromBucketURL(s3URL, sess)
		if err != nil {
			return nil, err

		}
	}
	return f, nil
}

func CopyFile(srcFS fs.FS, destFS afero.Fs) fs.WalkDirFunc {

	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			mkdirErr := destFS.MkdirAll(d.Name(), 0755)
			if mkdirErr != nil {
				return fmt.Errorf("failed to mkdir: %v", err)
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

func copyStaticFiles(srcFS fs.FS, srcPath string, destFS afero.Fs, destPath string) error {
	f := CopyFile(srcFS, destFS)
	return fs.WalkDir(srcFS, srcPath, f)
}

func GenerateIndexFiles(cfg Config) error {
	sess := session.Must(
		session.NewSessionWithOptions(
			session.Options{
				SharedConfigState: session.SharedConfigEnable,
			},
		),
	)

	s3Client := s3.New(sess, &aws.Config{
		DisableRestProtocolURICleaning: aws.Bool(true),
	})
	xray.AWS(s3Client.Client)

	startTime := time.Now()
	t, err := fetchBucketObjectTree(s3Client, cfg.Bucket, cfg.ObjectPrefix)
	endTime := time.Now()
	if err != nil {
		return err
	}

	log.Printf("fetchBucketObjectTree: duration:%v\n", (endTime.Sub(startTime)))
	// PrintTree(log.Writer(), t)

	var sp afero.Fs

	serverSideEncryption := &cfg.ServerSideEncryption
	bucketKeyEnabled := true
	if *serverSideEncryption == "" {
		serverSideEncryption = nil
		bucketKeyEnabled = false
	}

	cacheControl := fmt.Sprintf("max-age=%d", (time.Minute/time.Second)*5)

	fileProps := &aferos3.UploadedFileProperties{
		CacheControl:         &cacheControl,
		ServerSideEncryption: serverSideEncryption,
		BucketKeyEnabled:     &bucketKeyEnabled,
	}
	sp = aferos3.NewFs(cfg.Bucket, sess)
	sp.(*aferos3.Fs).FileProps = fileProps

	if cfg.LocalOutputDirectory != "" {
		// if we're testing locally
		o := afero.NewOsFs()
		mkdirErr := o.MkdirAll(cfg.LocalOutputDirectory, 0755)
		if mkdirErr != nil {
			return mkdirErr
		}
		sp = afero.NewBasePathFs(o, cfg.LocalOutputDirectory)
	}

	// select renderer
	renderer := renderObjectTreeAsMultiPage
	if cfg.IndexType == SinglePageIdentifier {
		renderer = renderObjectTreeAsSinglePage
	}
	// end select renderer

	// STart of load templates

	tmplFS, err := getFSFromS3URLOrDefault(cfg.TemplateBucketURL, sess, defaultTemplateFS)
	if err != nil {
		log.Fatalf("err: failed to load templates from bucket %v: %v", cfg.TemplateBucketURL, err)
	}

	startTime = time.Now()
	tmpl, err := loadTemplates(tmplFS)
	if err != nil {
		log.Fatalf("err: failed to load templates: %v", err)
	}
	endTime = time.Now()
	log.Printf("loadTemplates: duration:%v\n", endTime.Sub(startTime))
	// End of load templates

	staticFS, err := getFSFromS3URLOrDefault(cfg.StaticBucketURL, sess, defaultStaticFS)
	if err != nil {
		log.Fatalf("err: failed to load static assets from bucket %v: %v", cfg.StaticBucketURL, err)
	}

	// copy static
	startTime = time.Now()
	err = copyStaticFiles(staticFS, "static", sp, "static")
	if err != nil {
		log.Fatalf("err: failed to copy static files %v", err)
	}
	endTime = time.Now()
	log.Printf("copyStaticFiles: duration:%v\n", endTime.Sub(startTime))

	startTime = time.Now()
	err = renderer(t, tmpl, cfg.IndexTemplate, sp)
	endTime = time.Now()
	log.Printf("render: duration:%v\n", endTime.Sub(startTime))

	return renderer(t, tmpl, cfg.IndexTemplate, sp)
}

package main

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	aferos3 "github.com/fclairamb/afero-s3"

	//"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/spf13/afero"
)

//go:embed templates
var staticFS embed.FS

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

func renderObjectTreeAsSinglePage(objectTree *ObjectTree, tmpl *template.Template, templateName string, destFS afero.Fs) error {
	f, err := destFS.OpenFile(IndexFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	p := Page{
		Nonce:      "asdf",
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

func getTemplate(cfg Config, sess *session.Session) *template.Template {
	var templateFS fs.FS
	var err error
	templateFS = staticFS
	if cfg.TemplateBucketURL != nil {
		templateFS, err = IOFSFromS3URL(sess, cfg.TemplateBucketURL)
		if err != nil {
			log.Fatalf("err: failed to load FS from %v: %v", cfg.TemplateBucketURL.Redacted(), err)
		}
	}

	tmpl, err := loadTemplates(templateFS)
	if err != nil {
		log.Fatalf("err: failed to load templates: %v", err)
	}

	return tmpl
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

	t, err := fetchBucketObjectTree(s3Client, cfg.Bucket, cfg.ObjectPrefix)
	if err != nil {
		return err
	}

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

	renderer := renderObjectTreeAsMultiPage
	if cfg.IndexType == SinglePageIdentifier {
		renderer = renderObjectTreeAsSinglePage
	}

	tmpl := getTemplate(cfg, sess)

	return renderer(t, tmpl, cfg.IndexTemplate, sp)
}

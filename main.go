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

func fetchBucketObjectTree(s3Client *s3.S3, objectBucketName string) *ObjectTree {
	objInput := s3.ListObjectsInput{
		Bucket: &objectBucketName,
	}
	objects, err := s3Client.ListObjects(&objInput)
	if err != nil {
		fmt.Printf("err: %v", err)
	}
	t := CreateObjectTree(objects.Contents)
	return t
}

func renderObjectTreeAsSinglePage(objectTree *ObjectTree, tmpl *template.Template, templateName string, destFS afero.Fs) error {
	f, err := destFS.OpenFile("/index.html", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.ExecuteTemplate(f, templateName, objectTree)
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

type Config struct {
	SourceBucketName      string
	DestinationBucketName string
	TemplateBucketURL     *url.URL
	IndexType             string
	IndexTemplate         string
}

func parseConfigFromEnvironment() Config {
	var cfg Config

	var ok bool

	if cfg.SourceBucketName, ok = os.LookupEnv("SRC_BUCKET_NAME"); !ok {
		log.Fatal("err: SRC_BUCKET_NAME environment variable not set")
	}

	if cfg.DestinationBucketName, ok = os.LookupEnv("DEST_BUCKET_NAME"); !ok {
		cfg.DestinationBucketName = cfg.SourceBucketName
	}

	if cfg.IndexType, ok = os.LookupEnv("INDEX_TYPE"); !ok {
		cfg.IndexType = "multipage"
	} else {
		if cfg.IndexType != "multipage" && cfg.IndexType != "singlepage" {
			log.Fatalf("err: expected multipage or singlepage, found %v", cfg.IndexType)
		}
	}

	if cfg.IndexTemplate, ok = os.LookupEnv("INDEX_TEMPLATE"); !ok {
		cfg.IndexTemplate = fmt.Sprintf("%v.index.html.tmpl", cfg.IndexType)
	}

	var templateBucketURLString string

	if templateBucketURLString, ok = os.LookupEnv("TEMPLATE_BUCKET_URL"); ok {
		tmpURL, err := url.Parse(templateBucketURLString)
		if err != nil {
			log.Fatalf("err: unable to parse TEMPLATE_BUCKET_URL as URL: %v", err)
		}
		cfg.TemplateBucketURL = tmpURL
	}

	return cfg
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

	template, err := loadTemplates(templateFS)
	if err != nil {
		log.Fatalf("err: failed to load templates: %v", err)
	}

	return template
}

func main() {
	sess := session.Must(
		session.NewSessionWithOptions(
			session.Options{
				SharedConfigState: session.SharedConfigEnable,
			},
		),
	)

	cfg := parseConfigFromEnvironment()

	template := getTemplate(cfg, sess)

	s3Client := s3.New(sess, &aws.Config{
		DisableRestProtocolURICleaning: aws.Bool(true),
	})

	t := fetchBucketObjectTree(s3Client, cfg.SourceBucketName)

	var sp afero.Fs

	sp = aferos3.NewFs(cfg.DestinationBucketName, sess)

	// if we're testing locally
	o := afero.NewOsFs()
	o.MkdirAll("output", 0755)
	sp = afero.NewBasePathFs(o, "output")

	renderer := renderObjectTreeAsMultiPage
	if cfg.IndexType == "singlepage" {
		renderer = renderObjectTreeAsSinglePage
	}

	err := renderer(t, template, cfg.IndexTemplate, sp)
	if err != nil {
		fmt.Printf("err: %v\n", err)
	}
}

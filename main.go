package main

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	//"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

//go:embed templates/*
var static embed.FS

func ObjectBaseName(objKey string) string {
	parts := strings.Split(objKey, "/")
	return parts[len(parts)-1]
}

func loadTemplates() (*template.Template, error) {
	tmpl := template.New("")
	tplFuncMap := make(template.FuncMap)
	tplFuncMap["ObjectBaseName"] = ObjectBaseName
	tmpl.Funcs(tplFuncMap)
	tmpl, err := tmpl.ParseFS(static, "**/*")
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}

func renderSinglePageObjectTree(t *ObjectTree, out io.Writer) error {
	tmpl, err := loadTemplates()
	if err != nil {
		return err
	}
	return tmpl.ExecuteTemplate(out, "singlepage.index.html.tmpl", t)
}

func renderObjectTreeAsSinglePageWithWriter(objectTree *ObjectTree, out io.Writer) error {
	return renderSinglePageObjectTree(objectTree, out)
}

func fetchBucketObjectTree(objectBucketName string) *ObjectTree {
	sess := session.Must(
		session.NewSessionWithOptions(
			session.Options{
				SharedConfigState: session.SharedConfigEnable,
			},
		),
	)

	svc := s3.New(sess, &aws.Config{
		DisableRestProtocolURICleaning: aws.Bool(true),
	})

	objInput := s3.ListObjectsInput{
		Bucket: &objectBucketName,
	}
	objects, err := svc.ListObjects(&objInput)
	if err != nil {
		fmt.Printf("err: %v", err)
	}
	t := CreateObjectTree(objects.Contents)
	return t
}

func renderObjectTreeAsSinglePage(objectTree *ObjectTree, destinationPath string) error {
	indexPath := filepath.Join(destinationPath, "index.html")
	f, err := os.OpenFile(indexPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC	, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return renderObjectTreeAsSinglePageWithWriter(objectTree, f)
}

func renderBucketAsMultiPage(objectTree *ObjectTree, destinationPath string) error {
	tmpl, err := loadTemplates()
	if err != nil {
		return err
	}

	outputPath := filepath.Join(destinationPath, objectTree.DirName)
	outputFile := filepath.Join(outputPath, "index.html")

	err = os.MkdirAll(outputPath, 0755)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(outputFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	err = tmpl.ExecuteTemplate(f, "multipage.index.html.tmpl", objectTree)
	if err != nil {
		return err
	}
	for _, v := range objectTree.Children {
		err = renderBucketAsMultiPage(v, outputPath)

		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	var objectBucketName string
	var ok bool

	if objectBucketName, ok = os.LookupEnv("OBJECT_BUCKET_NAME"); !ok {
		log.Fatal("err: OBJECT_BUCKET_NAME environment variable not set")
	}

	t := fetchBucketObjectTree(objectBucketName)

	err := renderObjectTreeAsSinglePage(t, ".")
	if err != nil {
		fmt.Printf("err: %v", err)
	}

	err = renderBucketAsMultiPage(t, "multi")
	if err != nil {
		fmt.Printf("err: %v", err)
	}
}

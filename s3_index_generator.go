package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"os"
	"path"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/spf13/afero"

	//"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

//go:embed templates
var defaultTemplateFS embed.FS

//go:embed static
var defaultStaticFS embed.FS

var DioadIndexConfig = IndexConfig{
	ProductTagName:      "Dioad/Project",
	VersionTagName:      "Dioad/Version",
	ArchitectureTagName: "Dioad/Architecture",
	OSTagName:           "Dioad/OS",
}

type ObjectTreeConfig struct {
	PrefixToStrip string
	Exclusions    Exclusions
}

func fetchBucketObjectTree(ctx context.Context, bucket ObjectLister, objectPrefix string) (*ObjectTree, error) {
	//	func fetchBucketObjectTree(ctx context.Context, s3Client *s3.S3, objectBucketName string, objectPrefix string) (*ObjectTree, error) {
	objects, err := bucket.ListObjects(ctx, objectPrefix)
	if err != nil {
		return nil, err
	}

	cfg := ObjectTreeConfig{
		PrefixToStrip: objectPrefix,
		Exclusions: Exclusions{
			ExcludeKey("favicon.ico"),
			ExcludeKey("index.html"),
			ExcludePrefix("."),
			ExcludeSuffix("/"),
			ExcludeSuffix("/index.html"),
		},
	}

	t := NewObjectTreeWithObjects(cfg, objects)

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

type ObjectTreeIndexGenerator func(cfg IndexConfig, objectTree *ObjectTree) any
type ObjectTreeIndexIdentifier func(objectTree *ObjectTree) bool

func IndexForObjectTree(cfg IndexConfig, objectTree *ObjectTree) any {
	var index any
	if IsProductTree(objectTree) {
		index = NewProductIndexForObjectTree(cfg, objectTree)
	}
	if IsArchiveTree(objectTree) {
		index = NewArchiveIndexForObjectTree(cfg, objectTree)
	}
	if IsVersionTree(objectTree) {
		index = NewVersionIndexForObjectTree(cfg, objectTree)
	}
	return index
}

func writeObjectTreeIndex(cfg IndexConfig, objectTree *ObjectTree, destFS afero.Fs) error {
	var index any
	if IsProductTree(objectTree) {
		index = NewProductIndexForObjectTree(cfg, objectTree)
	}
	if IsArchiveTree(objectTree) {
		index = NewArchiveIndexForObjectTree(cfg, objectTree)
	}
	if IsVersionTree(objectTree) {
		index = NewVersionIndexForObjectTree(cfg, objectTree)
	}

	return writeIndexFile(index, destFS)
}

func writeIndexFile(index any, destFS afero.Fs) error {

	indexFile := path.Join("index.json")
	f, err := destFS.OpenFile(indexFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	jsonEncoder := json.NewEncoder(f)

	err = jsonEncoder.Encode(index)
	if err != nil {
		closeErr := f.Close()
		return fmt.Errorf("%w: %w", closeErr, err)
	}

	return f.Close()
}

func renderObjectTreeAsSinglePage(objectTree *ObjectTree, tmpl *template.Template, templateName string, destFS afero.Fs) error {
	f, err := destFS.OpenFile(IndexFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	p := Page{
		Nonce:      nonce(),
		ObjectTree: objectTree,
	}

	err = tmpl.ExecuteTemplate(f, templateName, p)
	if err != nil {
		closeErr := f.Close()
		return fmt.Errorf("%w: %w", closeErr, err)
	}

	return f.Close()
}

func renderObjectTreeAsMultiPage(objectTree *ObjectTree, tmpl *template.Template, templateName string, destFS afero.Fs) error {
	err := renderObjectTreeAsSinglePage(objectTree, tmpl, templateName, destFS)
	if err != nil {
		return err
	}

	for _, v := range objectTree.Children {
		v := v

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

	index := IndexForObjectTree(DioadIndexConfig, objectTree)

	err = writeIndexFile(index, destFS)

	return err
}

func s3Session() *session.Session {
	sess := session.Must(
		session.NewSessionWithOptions(
			session.Options{
				SharedConfigState: session.SharedConfigEnable,
			},
		),
	)
	return sess
}

func s3Client(sess *session.Session) *s3.S3 {
	client := s3.New(sess, &aws.Config{
		DisableRestProtocolURICleaning: aws.Bool(true),
	})
	xray.AWS(client.Client)

	return client
}

func GenerateIndexFiles(objectTree *ObjectTree, outputFS afero.Fs, tmpl *template.Template, indexTemplate string, indexType string) error {
	// select renderer
	renderer := renderObjectTreeAsMultiPage
	if indexType == SinglePageIdentifier {
		renderer = renderObjectTreeAsSinglePage
	}
	// end select renderer

	return renderer(objectTree, tmpl, indexTemplate, outputFS)
}

func CreateObjectTree(objectLister ObjectLister, objectPrefix string) (*ObjectTree, error) {
	objectTreeCfg := ObjectTreeConfig{
		PrefixToStrip: objectPrefix,
		Exclusions: Exclusions{
			ExcludeKey("favicon.ico"),
			ExcludeKey("index.html"),
			ExcludePrefix("."),
			ExcludeSuffix("/"),
			ExcludeSuffix("/index.html"),
		},
	}

	objectTree := NewRootObjectTree(objectTreeCfg)
	err := objectTree.AddObjectsFromLister(objectLister)
	if err != nil {
		return nil, err
	}
	return objectTree, nil
}

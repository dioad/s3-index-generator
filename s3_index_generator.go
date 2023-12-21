package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"path"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

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

type IndexRenderer struct {
	IndexFile string
	Render    func(io.Writer, *ObjectTree) error
}

func JSONRenderer(config IndexConfig) IndexRenderer {
	return IndexRenderer{
		IndexFile: "index.json",
		Render: func(stream io.Writer, objectTree *ObjectTree) error {
			index := IndexForObjectTree(config, objectTree)

			jsonEncoder := json.NewEncoder(stream)

			return jsonEncoder.Encode(index)
		},
	}
}

func HTMLRenderer(tmpl *template.Template, templateName string) IndexRenderer {
	return IndexRenderer{
		IndexFile: "index.html",
		Render: func(stream io.Writer, objectTree *ObjectTree) error {
			p := Page{
				Nonce:      nonce(),
				ObjectTree: objectTree,
			}

			return tmpl.ExecuteTemplate(stream, templateName, p)
		},
	}
}

func renderObjectTreeToFile(objectTree *ObjectTree, fileRenderer IndexRenderer, destFS afero.Fs) error {
	indexFile := path.Join(fileRenderer.IndexFile)
	f, err := destFS.OpenFile(indexFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	err = fileRenderer.Render(f, objectTree)
	if err != nil {
		writerErr := f.Close()
		return fmt.Errorf("failed to render index file: %w, failed to close file: %w", err, writerErr)
	}

	return f.Close()
}

type IndexRenderers []IndexRenderer

func (r IndexRenderers) Render(destFS afero.Fs, objectTree *ObjectTree) error {
	errGroup := errgroup.Group{}
	for _, renderer := range r {
		renderer := renderer
		errGroup.Go(func() error {
			return renderObjectTreeToFile(objectTree, renderer, destFS)
		})
	}
	return errGroup.Wait()
}

func RenderObjectTree(objectTree *ObjectTree, renderers IndexRenderers, destFS afero.Fs, recursive bool) error {
	errGroup := errgroup.Group{}
	errGroup.SetLimit(10)

	err := destFS.MkdirAll(objectTree.DirName, 0755)
	if err != nil {
		return err
	}

	thisFS := afero.NewBasePathFs(destFS, objectTree.DirName)

	errGroup.Go(func() error {
		return renderers.Render(thisFS, objectTree)
	})

	if recursive {
		for _, v := range objectTree.Children {
			v := v

			errGroup.Go(func() error {
				return RenderObjectTree(v, renderers, thisFS, recursive)
			})
		}
	}

	return errGroup.Wait()
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

	renderers := IndexRenderers{
		HTMLRenderer(tmpl, indexTemplate),
		JSONRenderer(DioadIndexConfig),
	}

	// select renderer
	recursive := true
	if indexType == SinglePageIdentifier {
		recursive = false
	}
	// end select renderer

	return RenderObjectTree(objectTree, renderers, outputFS, recursive)
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

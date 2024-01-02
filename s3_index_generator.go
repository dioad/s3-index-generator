package main

import (
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

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
)

//go:embed templates
var defaultTemplateFS embed.FS

//go:embed static
var defaultStaticFS embed.FS

var DioadIndexConfig = IndexConfig{
	KeyExtractions: ReleaseDetailKeyExtractions{
		DefaultReleaseInfoKeyExtractor,
	},
}

type ObjectTreeConfig struct {
	PrefixToStrip string
	Exclusions    Exclusions
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

func JSONIndexRenderer(config IndexConfig) IndexRenderer {
	return IndexRenderer{
		IndexFile: "index.json",
		Render: func(stream io.Writer, objectTree *ObjectTree) error {
			// New Index
			index := IndexForObjectTree(config, objectTree)

			jsonEncoder := json.NewEncoder(stream)

			return jsonEncoder.Encode(index)
		},
	}
}

func HTMLIndexRenderer(tmpl *template.Template, templateName string) IndexRenderer {
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

func RenderObjectTreeIndexFile(objectTree *ObjectTree, fileRenderer IndexRenderer, destFS afero.Fs) error {
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
			return RenderObjectTreeIndexFile(objectTree, renderer, destFS)
		})
	}
	return errGroup.Wait()
}

func RenderWalker(destFS afero.Fs, renderers IndexRenderers) ObjectTreeWalker {
	return func(objectTree *ObjectTree) error {
		err := destFS.MkdirAll(objectTree.FullPath, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %v: %w", objectTree.FullPath, err)
		}
		thisFS := afero.NewBasePathFs(destFS, objectTree.FullPath)

		err = renderers.Render(thisFS, objectTree)
		if err != nil {
			return fmt.Errorf("failed to render object tree indexes for %v: %w", objectTree.FullPath, err)
		}
		return nil
	}
}

func RenderObjectTreeIndexes(objectTree *ObjectTree, renderers IndexRenderers, destFS afero.Fs, recursive bool) error {
	walker := RenderWalker(destFS, renderers)

	return objectTree.Walk(walker, recursive, true)
}

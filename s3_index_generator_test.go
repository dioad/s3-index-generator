package main

import (
	"context"
	"embed"
	"encoding/json"
	"io"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/spf13/afero"
)

//go:embed templates
var testTemplateFS embed.FS

//go:embed static
var testStaticFS embed.FS

func TestLoadTemplates(t *testing.T) {
	tmpl, err := loadTemplates(testTemplateFS)
	if err != nil {
		t.Errorf("loadTemplates() error = %v", err)
		return
	}
	if tmpl == nil {
		t.Errorf("loadTemplates() = nil, want non-nil")
	}
}

func TestCopyStaticFiles(t *testing.T) {
	srcFS := testStaticFS
	destFS := afero.NewMemMapFs()

	err := CopyFilesFromSubPath(destFS, srcFS, "static")
	if err != nil {
		t.Errorf("CopyFilesFromSubPath() error = %v", err)
		return
	}
}

func TestRenderObjectTreeIndexes_JSON(t *testing.T) {
	objects := []Object{
		simpleObject("connect/1.0.0/connect_linux_amd64.zip"),
		simpleObject("connect/1.0.0/connect_darwin_amd64.zip"),
	}
	tree := NewObjectTreeWithObjects(ObjectTreeConfig{}, objects)

	renderer := JSONIndexRenderer(DioadIndexConfig)
	destFS := afero.NewMemMapFs()

	err := RenderObjectTreeIndexes(tree, IndexRenderers{renderer}, destFS, true)
	if err != nil {
		t.Fatalf("RenderObjectTreeIndexes() error = %v", err)
	}

	// Verify that index.json exists at the version level and is valid JSON.
	f, err := destFS.Open("/connect/1.0.0/index.json")
	if err != nil {
		t.Fatalf("expected /connect/1.0.0/index.json to exist: %v", err)
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("reading index.json: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("index.json is not valid JSON: %v\ncontent: %s", err, data)
	}
}

func TestRenderObjectTreeIndexes_HTML(t *testing.T) {
	tmpl, err := loadTemplates(testTemplateFS)
	if err != nil {
		t.Fatalf("loadTemplates() error = %v", err)
	}

	objects := []Object{
		simpleObject("connect/1.0.0/connect_linux_amd64.zip"),
	}
	tree := NewObjectTreeWithObjects(ObjectTreeConfig{}, objects)

	renderer := HTMLIndexRenderer(tmpl, "multipage.index.html.tmpl")
	destFS := afero.NewMemMapFs()

	err = RenderObjectTreeIndexes(tree, IndexRenderers{renderer}, destFS, true)
	if err != nil {
		t.Fatalf("RenderObjectTreeIndexes() error = %v", err)
	}

	// Verify HTML was generated at the root level.
	f, err := destFS.Open("/index.html")
	if err != nil {
		t.Fatalf("expected /index.html to exist: %v", err)
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}
	if len(data) == 0 {
		t.Error("index.html is empty")
	}
}

func TestRenderObjectTreeIndexes_JSONAndHTML(t *testing.T) {
	tmpl, err := loadTemplates(testTemplateFS)
	if err != nil {
		t.Fatalf("loadTemplates() error = %v", err)
	}

	objects := []Object{simpleObject("product/2.0.0/product_linux_amd64.tar.gz")}
	tree := NewObjectTreeWithObjects(ObjectTreeConfig{}, objects)

	renderers := IndexRenderers{
		JSONIndexRenderer(DioadIndexConfig),
		HTMLIndexRenderer(tmpl, "multipage.index.html.tmpl"),
	}
	destFS := afero.NewMemMapFs()

	err = RenderObjectTreeIndexes(tree, renderers, destFS, true)
	if err != nil {
		t.Fatalf("RenderObjectTreeIndexes() error = %v", err)
	}

	for _, path := range []string{"/index.json", "/index.html", "/product/index.json", "/product/index.html"} {
		if _, err := destFS.Stat(path); err != nil {
			t.Errorf("expected %s to exist: %v", path, err)
		}
	}
}

func TestHandleRequest_EmptyRecords(t *testing.T) {
	handler := HandleRequest(nil, Config{})
	err := handler(context.Background(), events.S3Event{Records: []events.S3EventRecord{}})
	if err != nil {
		t.Errorf("expected nil error for empty records, got %v", err)
	}
}

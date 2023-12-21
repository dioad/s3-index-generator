package main

import (
	"context"
	"embed"
	"testing"

	"github.com/aws/aws-sdk-go/service/s3"
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

type testBucket struct{}

func (b *testBucket) ListObjects(ctx context.Context, key string) ([]Object, error) {
	return []Object{
		&object{
			obj: &s3.Object{Key: stringToPointer("testKey")},
			tags: map[string]string{
				"testTagKey": "testTagValue",
			},
		},
	}, nil
}

//func TestCopyFile(t *testing.T) {
//	srcFS := afero.NewMemMapFs()
//	destFS := afero.NewMemMapFs()
//
//	// Create a file in the source filesystem
//	afero.WriteFile(srcFS, "testFile", []byte("testContent"), 0644)
//
//	// Copy the file to the destination filesystem
//	f := CopyFile(afero.NewIOFS(srcFS), destFS)
//	err := f("testFile", fs.DirEntry{}, nil)
//	if err != nil {
//		t.Errorf("CopyFile() error = %v", err)
//		return
//	}
//
//	// Check if the file exists in the destination filesystem
//	exists, err := afero.Exists(destFS, "testFile")
//	if err != nil || !exists {
//		t.Errorf("CopyFile() failed, file not copied")
//	}
//}

func TestCopyStaticFiles(t *testing.T) {
	srcFS := testStaticFS
	destFS := afero.NewMemMapFs()

	err := CopyFilesFromSubPath(destFS, srcFS, "static")
	if err != nil {
		t.Errorf("CopyFilesFromSubPath() error = %v", err)
		return
	}
}

//func TestGenerateIndexFiles(t *testing.T) {
//	ctx := context.Background()
//	cfg := Config{
//		Bucket:        "testBucket",
//		ObjectPrefix:  "testPrefix",
//		IndexType:     "testIndexType",
//		IndexTemplate: "testTemplate",
//		TemplateBucketURL: &url.URL{
//			Scheme: "s3",
//			Host:   "testBucket",
//			Path:   "testPath",
//		},
//		StaticBucketURL: &url.URL{
//			Scheme: "s3",
//			Host:   "testBucket",
//			Path:   "testPath",
//		},
//	}
//
//	err := GenerateIndexFiles(ctx, cfg)
//	if err != nil {
//		t.Errorf("GenerateIndexFiles() error = %v", err)
//		return
//	}
//}

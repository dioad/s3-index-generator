package main

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/spf13/afero"
)

func TestHumanizeBytes(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1048576, "1.0 MiB"},
		{5 * 1024 * 1024 * 1024, "5.0 GiB"},
	}

	for _, tt := range tests {
		if got := humanizeBytes(tt.size); got != tt.want {
			t.Errorf("humanizeBytes(%d) = %q, want %q", tt.size, got, tt.want)
		}
	}
}

// signedMapFS is an in-memory file system with a settable signature that counts how often it is read.
type signedMapFS struct {
	files fstest.MapFS
	sig   string
	opens int
}

func (f *signedMapFS) Open(name string) (fs.File, error) {
	f.opens++

	return f.files.Open(name)
}

//nolint:unparam // signature is fixed by signedFS
func (f *signedMapFS) signature() (string, error) { return f.sig, nil }

func TestLoadCachedTemplates(t *testing.T) {
	t.Cleanup(resetCaches)
	resetCaches()

	src := &signedMapFS{files: fstest.MapFS{"templates/a.tmpl": {Data: []byte("one")}}, sig: "v1"}

	first, err := loadCachedTemplates(src)
	if err != nil {
		t.Fatal(err)
	}
	opensAfterFirst := src.opens

	second, err := loadCachedTemplates(src)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || src.opens != opensAfterFirst {
		t.Errorf("unchanged signature should reuse the parsed templates without reading the source (opens %d -> %d)", opensAfterFirst, src.opens)
	}

	src.files["templates/a.tmpl"] = &fstest.MapFile{Data: []byte("two")}
	src.sig = "v2"

	third, err := loadCachedTemplates(src)
	if err != nil {
		t.Fatal(err)
	}
	if third == first || src.opens == opensAfterFirst {
		t.Error("changed signature should reload the templates")
	}

	resetCaches()
	if fourth, _ := loadCachedTemplates(src); fourth == third {
		t.Error("resetCaches should force a reload")
	}
}

func TestCopyStaticOnce(t *testing.T) {
	t.Cleanup(resetCaches)
	resetCaches()

	src := &signedMapFS{files: fstest.MapFS{"static/style.css": {Data: []byte("a{}")}}, sig: "v1"}
	dest := afero.NewMemMapFs()

	if err := copyStaticOnce(dest, src); err != nil {
		t.Fatal(err)
	}
	opens := src.opens
	if exists, _ := afero.Exists(dest, "static/style.css"); !exists {
		t.Fatal("expected static/style.css to be copied")
	}

	if err := copyStaticOnce(dest, src); err != nil {
		t.Fatal(err)
	}
	if src.opens != opens {
		t.Error("unchanged assets copied to the same destination should not be copied again")
	}

	if err := copyStaticOnce(afero.NewMemMapFs(), src); err != nil {
		t.Fatal(err)
	}
	if src.opens == opens {
		t.Error("a different destination should be copied to")
	}

	opens = src.opens
	src.sig = "v2"
	if err := copyStaticOnce(afero.NewMemMapFs(), src); err != nil {
		t.Fatal(err)
	}
	if src.opens == opens {
		t.Error("changed assets should be copied again")
	}
}

func TestS3ReadFSSignature(t *testing.T) {
	etag := "a"
	client := &fakeS3Read{
		listObjects: func(in *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
			if got := *in.Prefix; got != "pfx/" {
				t.Errorf("Prefix = %q, want pfx/", got)
			}

			return &s3.ListObjectsV2Output{Contents: []s3types.Object{{Key: aws.String("pfx/templates/a.tmpl"), ETag: aws.String(etag), Size: aws.Int64(3)}}}, nil
		},
	}
	f := newS3ReadFS(client, "bucket", "pfx")

	before, err := f.signature()
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := f.signature(); again != before {
		t.Error("signature should be stable for unchanged objects")
	}

	etag = "b"
	if after, _ := f.signature(); after == before {
		t.Error("signature should change when an ETag changes")
	}
}

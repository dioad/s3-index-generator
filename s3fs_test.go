package main

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// fakeS3Read implements s3ReadAPI with canned responses.
type fakeS3Read struct {
	ctxs        []context.Context // the context of every call, in order
	getObject   func(*s3.GetObjectInput) (*s3.GetObjectOutput, error)
	listObjects func(*s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error)
}

func (f *fakeS3Read) GetObject(ctx context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.ctxs = append(f.ctxs, ctx)

	return f.getObject(in)
}

func (f *fakeS3Read) HeadObject(ctx context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	f.ctxs = append(f.ctxs, ctx)

	return nil, errors.New("unexpected HeadObject")
}

func (f *fakeS3Read) ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	f.ctxs = append(f.ctxs, ctx)

	if f.listObjects == nil {
		return nil, errors.New("unexpected ListObjectsV2")
	}

	return f.listObjects(in)
}

func TestS3ReadFSOpen(t *testing.T) {
	accessDenied := errors.New("operation error S3: GetObject, https response error StatusCode: 403, AccessDenied")

	tests := []struct {
		name     string
		getErr   error
		body     string
		wantDir  bool
		wantErr  error
		wantRead string
	}{
		{name: "object is returned as a file", body: "hello", wantRead: "hello"},
		{name: "missing key falls back to a directory", getErr: &s3types.NoSuchKey{}, wantDir: true},
		{name: "wrapped missing key falls back to a directory", getErr: errors.Join(errors.New("op"), &s3types.NoSuchKey{}), wantDir: true},
		{name: "other errors are surfaced", getErr: accessDenied, wantErr: accessDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeS3Read{getObject: func(in *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
				if got, want := *in.Key, "pfx/templates/a.tmpl"; got != want {
					t.Errorf("GetObject key = %q, want %q", got, want)
				}
				if tt.getErr != nil {
					return nil, tt.getErr
				}

				return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			}}

			f, err := newS3ReadFS(context.Background(), client, "bucket", "pfx").Open("templates/a.tmpl")

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Open() error = %v, want %v", err, tt.wantErr)
				}
				if _, ok := errors.AsType[*fs.PathError](err); !ok {
					t.Errorf("Open() error = %T, want *fs.PathError", err)
				}

				return
			}
			if err != nil {
				t.Fatalf("Open() unexpected error: %v", err)
			}

			if _, isDir := f.(*s3DirFile); isDir != tt.wantDir {
				t.Fatalf("Open() returned dir = %v, want %v", isDir, tt.wantDir)
			}
			if tt.wantDir {
				return
			}

			got, err := io.ReadAll(f)
			if err != nil || string(got) != tt.wantRead {
				t.Errorf("read = %q, %v; want %q", got, err, tt.wantRead)
			}
		})
	}
}

type ctxKey struct{}

// TestS3ReadFSUsesCallerContext checks that every S3 call made by the file system carries the context it was
// built with, so a cancelled or expired invocation stops in-flight reads.
func TestS3ReadFSUsesCallerContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKey{}, "invocation")
	client := &fakeS3Read{
		getObject: func(*s3.GetObjectInput) (*s3.GetObjectOutput, error) { return nil, &s3types.NoSuchKey{} },
		listObjects: func(*s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{}, nil
		},
	}
	f := newS3ReadFS(ctx, client, "bucket", "pfx")

	if _, err := f.Open("templates/a.tmpl"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ReadDir("templates"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.signature(); err != nil {
		t.Fatal(err)
	}
	_, _ = f.Stat("templates/a.tmpl") // fails in the fake (HeadObject is unexpected); only the context matters here

	// Open: 1, ReadDir: 1, signature: 1, Stat: a HeadObject then a directory listing.
	if len(client.ctxs) != 5 {
		t.Fatalf("expected 5 S3 calls, got %d", len(client.ctxs))
	}
	for i, got := range client.ctxs {
		if got.Value(ctxKey{}) != "invocation" {
			t.Errorf("call %d did not use the file system's context", i)
		}
	}
}

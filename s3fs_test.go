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
	getObject func(*s3.GetObjectInput) (*s3.GetObjectOutput, error)
}

func (f *fakeS3Read) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return f.getObject(in)
}

func (f *fakeS3Read) HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	return nil, errors.New("unexpected HeadObject")
}

func (f *fakeS3Read) ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	return nil, errors.New("unexpected ListObjectsV2")
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

			f, err := newS3ReadFS(client, "bucket", "pfx").Open("templates/a.tmpl")

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

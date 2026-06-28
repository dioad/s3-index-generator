package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/spf13/afero"
)

// --------------------------------------------------------------------------
// S3 write filesystem (afero.Fs) — used for index output to S3
// --------------------------------------------------------------------------

// S3FileProps configures metadata applied to uploaded objects.
type S3FileProps struct {
	CacheControl         *string
	ServerSideEncryption s3types.ServerSideEncryption
	BucketKeyEnabled     *bool
}

type s3WriteFS struct {
	client *s3.Client
	bucket string
	props  *S3FileProps
}

func (f *s3WriteFS) Name() string                                         { return "s3" }
func (f *s3WriteFS) Mkdir(name string, _ os.FileMode) error               { return nil }
func (f *s3WriteFS) MkdirAll(_ string, _ os.FileMode) error               { return nil }
func (f *s3WriteFS) Chmod(_ string, _ os.FileMode) error                  { return nil }
func (f *s3WriteFS) Chown(_ string, _, _ int) error                       { return nil }
func (f *s3WriteFS) Chtimes(_ string, _, _ time.Time) error               { return nil }
func (f *s3WriteFS) Remove(_ string) error                                 { return nil }
func (f *s3WriteFS) RemoveAll(_ string) error                              { return nil }
func (f *s3WriteFS) Rename(_, _ string) error                             { return nil }
func (f *s3WriteFS) Stat(name string) (os.FileInfo, error) {
	return nil, &os.PathError{Op: "stat", Path: name, Err: fs.ErrPermission}
}
func (f *s3WriteFS) Open(name string) (afero.File, error) {
	return nil, &os.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
}

func (f *s3WriteFS) Create(name string) (afero.File, error) {
	return f.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
}

func (f *s3WriteFS) OpenFile(name string, _ int, _ os.FileMode) (afero.File, error) {
	key := strings.TrimPrefix(name, "/")
	return &s3WriteFile{
		client: f.client,
		bucket: f.bucket,
		key:    key,
		props:  f.props,
		buf:    &bytes.Buffer{},
	}, nil
}

// s3WriteFile buffers writes and uploads to S3 on Close.
type s3WriteFile struct {
	client *s3.Client
	bucket string
	key    string
	props  *S3FileProps
	buf    *bytes.Buffer
	closed bool
}

func (f *s3WriteFile) Name() string { return "/" + f.key }

func (f *s3WriteFile) Write(p []byte) (int, error) {
	if f.closed {
		return 0, os.ErrClosed
	}
	return f.buf.Write(p)
}

func (f *s3WriteFile) WriteString(s string) (int, error) {
	return f.Write([]byte(s))
}

func (f *s3WriteFile) Close() error {
	if f.closed {
		return os.ErrClosed
	}
	f.closed = true

	contentType := mime.TypeByExtension(path.Ext(f.key))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	data := f.buf.Bytes()
	input := &s3.PutObjectInput{
		Bucket:      &f.bucket,
		Key:         aws.String(f.key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}

	if f.props != nil {
		input.CacheControl = f.props.CacheControl
		input.BucketKeyEnabled = f.props.BucketKeyEnabled
		if f.props.ServerSideEncryption != "" {
			input.ServerSideEncryption = f.props.ServerSideEncryption
		}
	}

	_, err := f.client.PutObject(context.Background(), input)
	if err != nil {
		return fmt.Errorf("s3 upload %s: %w", f.key, err)
	}
	return nil
}

func (f *s3WriteFile) Read(_ []byte) (int, error) {
	return 0, &os.PathError{Op: "read", Path: f.key, Err: fs.ErrPermission}
}
func (f *s3WriteFile) ReadAt(_ []byte, _ int64) (int, error) {
	return 0, &os.PathError{Op: "readat", Path: f.key, Err: fs.ErrPermission}
}
func (f *s3WriteFile) WriteAt(p []byte, off int64) (int, error) {
	return 0, &os.PathError{Op: "writeat", Path: f.key, Err: fs.ErrPermission}
}
func (f *s3WriteFile) Seek(_ int64, _ int) (int64, error) {
	return 0, &os.PathError{Op: "seek", Path: f.key, Err: fs.ErrPermission}
}
func (f *s3WriteFile) Readdir(_ int) ([]os.FileInfo, error) {
	return nil, &os.PathError{Op: "readdir", Path: f.key, Err: fs.ErrPermission}
}
func (f *s3WriteFile) Readdirnames(_ int) ([]string, error) {
	return nil, &os.PathError{Op: "readdirnames", Path: f.key, Err: fs.ErrPermission}
}
func (f *s3WriteFile) Stat() (os.FileInfo, error) {
	return nil, &os.PathError{Op: "stat", Path: f.key, Err: fs.ErrPermission}
}
func (f *s3WriteFile) Sync() error            { return nil }
func (f *s3WriteFile) Truncate(_ int64) error { return nil }

// --------------------------------------------------------------------------
// S3 read filesystem (fs.FS) — used for loading templates/static from S3
// --------------------------------------------------------------------------

type s3ReadFS struct {
	client *s3.Client
	bucket string
	prefix string // base prefix within bucket, no leading/trailing slashes
}

func newS3ReadFS(client *s3.Client, bucket, prefix string) *s3ReadFS {
	return &s3ReadFS{
		client: client,
		bucket: bucket,
		prefix: strings.Trim(prefix, "/"),
	}
}

func (f *s3ReadFS) fullKey(name string) string {
	if f.prefix == "" {
		return name
	}
	return f.prefix + "/" + name
}

// Open implements fs.FS.
func (f *s3ReadFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if name == "." {
		return &s3DirFile{fs: f, name: "."}, nil
	}

	key := f.fullKey(name)
	resp, err := f.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: &f.bucket,
		Key:    aws.String(key),
	})
	if err != nil {
		// Treat as a directory — ReadDir will determine if it actually exists.
		return &s3DirFile{fs: f, name: name}, nil
	}

	return &s3ReadFile{
		body: resp.Body,
		info: &s3FileInfo{
			name: path.Base(name),
			size: aws.ToInt64(resp.ContentLength),
		},
	}, nil
}

// ReadDir implements fs.ReadDirFS.
func (f *s3ReadFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}

	var s3Prefix string
	if name == "." {
		if f.prefix != "" {
			s3Prefix = f.prefix + "/"
		}
	} else {
		s3Prefix = f.fullKey(name) + "/"
	}

	var entries []fs.DirEntry
	var continuationToken *string

	for {
		output, err := f.client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
			Bucket:            &f.bucket,
			Prefix:            aws.String(s3Prefix),
			Delimiter:         aws.String("/"),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
		}

		for _, cp := range output.CommonPrefixes {
			if cp.Prefix == nil {
				continue
			}
			dirName := strings.TrimSuffix(strings.TrimPrefix(*cp.Prefix, s3Prefix), "/")
			if dirName != "" {
				entries = append(entries, &s3DirEntry{name: dirName, isDir: true})
			}
		}

		for _, obj := range output.Contents {
			if obj.Key == nil {
				continue
			}
			fileName := strings.TrimPrefix(*obj.Key, s3Prefix)
			if fileName == "" || strings.Contains(fileName, "/") {
				continue
			}
			entries = append(entries, &s3DirEntry{
				name:  fileName,
				isDir: false,
				size:  aws.ToInt64(obj.Size),
				mtime: aws.ToTime(obj.LastModified),
			})
		}

		if !aws.ToBool(output.IsTruncated) {
			break
		}
		continuationToken = output.NextContinuationToken
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	return entries, nil
}

// Stat implements fs.StatFS.
func (f *s3ReadFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	if name == "." {
		return &s3FileInfo{name: ".", mode: fs.ModeDir}, nil
	}

	key := f.fullKey(name)

	resp, err := f.client.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: &f.bucket,
		Key:    aws.String(key),
	})
	if err == nil {
		return &s3FileInfo{
			name:  path.Base(name),
			size:  aws.ToInt64(resp.ContentLength),
			mtime: aws.ToTime(resp.LastModified),
		}, nil
	}

	// Not a file — check if it's a directory prefix.
	listResp, listErr := f.client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket:  &f.bucket,
		Prefix:  aws.String(key + "/"),
		MaxKeys: aws.Int32(1),
	})
	if listErr != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: listErr}
	}
	if len(listResp.Contents) > 0 || len(listResp.CommonPrefixes) > 0 {
		return &s3FileInfo{name: path.Base(name), mode: fs.ModeDir}, nil
	}

	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

// --------------------------------------------------------------------------
// Shared helper types for s3ReadFS
// --------------------------------------------------------------------------

type s3FileInfo struct {
	name  string
	size  int64
	mtime time.Time
	mode  fs.FileMode
}

func (fi *s3FileInfo) Name() string       { return fi.name }
func (fi *s3FileInfo) Size() int64        { return fi.size }
func (fi *s3FileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi *s3FileInfo) ModTime() time.Time { return fi.mtime }
func (fi *s3FileInfo) IsDir() bool        { return fi.mode.IsDir() }
func (fi *s3FileInfo) Sys() any           { return nil }

type s3DirEntry struct {
	name  string
	isDir bool
	size  int64
	mtime time.Time
}

func (e *s3DirEntry) Name() string { return e.name }
func (e *s3DirEntry) IsDir() bool  { return e.isDir }
func (e *s3DirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}
func (e *s3DirEntry) Info() (fs.FileInfo, error) {
	return &s3FileInfo{
		name:  e.name,
		size:  e.size,
		mtime: e.mtime,
		mode:  e.Type(),
	}, nil
}

// s3ReadFile wraps a GetObject response body.
type s3ReadFile struct {
	body io.ReadCloser
	info *s3FileInfo
}

func (f *s3ReadFile) Read(p []byte) (int, error)      { return f.body.Read(p) }
func (f *s3ReadFile) Close() error                    { return f.body.Close() }
func (f *s3ReadFile) Stat() (fs.FileInfo, error)      { return f.info, nil }
func (f *s3ReadFile) ReadDir(_ int) ([]fs.DirEntry, error) {
	return nil, &fs.PathError{Op: "readdir", Path: f.info.Name(), Err: fs.ErrInvalid}
}

// s3DirFile is the fs.File returned for directory paths.
type s3DirFile struct {
	fs      *s3ReadFS
	name    string
	entries []fs.DirEntry
	pos     int
}

func (f *s3DirFile) Read(_ []byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: f.name, Err: fs.ErrInvalid}
}
func (f *s3DirFile) Close() error                { return nil }
func (f *s3DirFile) Stat() (fs.FileInfo, error)  { return &s3FileInfo{name: path.Base(f.name), mode: fs.ModeDir}, nil }

func (f *s3DirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if f.entries == nil {
		var err error
		f.entries, err = f.fs.ReadDir(f.name)
		if err != nil {
			return nil, err
		}
	}
	if n <= 0 {
		entries := f.entries[f.pos:]
		f.pos = len(f.entries)
		return entries, nil
	}
	if f.pos >= len(f.entries) {
		return nil, io.EOF
	}
	end := f.pos + n
	if end > len(f.entries) {
		end = len(f.entries)
	}
	entries := f.entries[f.pos:end]
	f.pos = end
	return entries, nil
}

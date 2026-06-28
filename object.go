package main

import (
	"context"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Object interface {
	Key() string
	LastModified() time.Time
	Size() int64
	BaseName() string
	Tags() map[string]string
	SetTags(tags map[string]string)
}

type object struct {
	key          string
	lastModified time.Time
	size         int64
	tags         map[string]string
}

func (o *object) Key() string             { return o.key }
func (o *object) LastModified() time.Time { return o.lastModified }
func (o *object) Size() int64             { return o.size }
func (o *object) BaseName() string        { return filepath.Base(o.key) }
func (o *object) Tags() map[string]string { return o.tags }
func (o *object) SetTags(tags map[string]string) {
	o.tags = tags
}

func NewObject(obj s3types.Object) Object {
	return &object{
		key:          aws.ToString(obj.Key),
		lastModified: aws.ToTime(obj.LastModified),
		size:         aws.ToInt64(obj.Size),
	}
}

type ObjectListerFunc func(ctx context.Context, prefix string) ([]Object, error)

type ObjectLister interface {
	ListObjects(ctx context.Context, prefix string) ([]Object, error)
}

type ObjectListerWithTags interface {
	ListObjectsWithTags(ctx context.Context, prefix string) ([]Object, error)
}

type ObjectTagFetcher interface {
	FetchObjectTags(ctx context.Context, key string) (map[string]string, error)
}

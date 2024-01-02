package main

import (
	"context"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
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
	obj  *s3.Object
	tags map[string]string
}

func (o *object) Key() string {
	if o.obj == nil || o.obj.Key == nil {
		return ""
	}
	return *o.obj.Key
}

func (o *object) LastModified() time.Time {
	if o.obj == nil || o.obj.LastModified == nil {
		return time.Time{}
	}
	return *o.obj.LastModified
}

func (o *object) Size() int64 {
	if o.obj == nil || o.obj.Size == nil {
		return 0
	}
	return *o.obj.Size
}

func (o *object) BaseName() string {
	return filepath.Base(o.Key())
}

func (o *object) Tags() map[string]string {
	return o.tags
}

func (o *object) SetTags(tags map[string]string) {
	o.tags = tags
}

func NewObject(obj *s3.Object) Object {
	return &object{
		obj:  obj,
		tags: make(map[string]string),
	}
}

func NewObjectWithTags(obj *s3.Object, tags map[string]string) Object {
	o := &object{
		obj:  obj,
		tags: tags,
	}
	return o
}

type ObjectTagSetter interface {
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

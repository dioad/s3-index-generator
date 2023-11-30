package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
	"golang.org/x/sync/errgroup"
)

type Object struct {
	obj  *s3.Object
	Tags map[string]string
}

func (o *Object) Key() string {
	if o.obj == nil || o.obj.Key == nil {
		return ""
	}
	return *o.obj.Key
}

func (o *Object) LastModified() time.Time {
	if o.obj == nil || o.obj.LastModified == nil {
		return time.Time{}
	}
	return *o.obj.LastModified
}

func (o *Object) Size() int64 {
	if o.obj == nil || o.obj.Size == nil {
		return 0
	}
	return *o.obj.Size
}

func (o *Object) BaseName() string {
	return filepath.Base(o.Key())
}

func (o *Object) AddTagSet(t []*s3.Tag) {
	o.Tags = make(map[string]string)
	for _, tag := range t {
		o.Tags[*tag.Key] = *tag.Value
	}
}

func NewObject(obj *s3.Object) *Object {
	return &Object{
		obj: obj,
	}
}

func (o *Object) IndexEntry(cfg IndexConfig) *IndexEntry {
	if _, exists := o.Tags[cfg.VersionTagName]; !exists {
		return nil
	}

	return NewIndexEntry(cfg, o)
}

func NewObjectWithTags(obj *s3.Object, tags []*s3.Tag) *Object {
	o := NewObject(obj)
	o.AddTagSet(tags)
	return o
}

// FetchObjectsWithContext fetches objects from S3 with a context.
func FetchObjectsWithContext(ctx context.Context, s3Client *s3.S3, bucketName string, prefix string) ([]*Object, error) {
	objInput := s3.ListObjectsInput{
		Bucket: &bucketName,
		Prefix: &prefix,
	}

	items := make([]*Object, 0)

	err := s3Client.ListObjectsPagesWithContext(ctx, &objInput, func(page *s3.ListObjectsOutput, lastPage bool) bool {
		pageItems, err := fetchObjectsAndTags(ctx, s3Client, bucketName, page)
		if err != nil {
			fmt.Printf("err: %v\n", err)
		}
		items = append(items, pageItems...)
		return true
	})

	return items, err
}

func fetchObjectTags(ctx context.Context, s3Client *s3.S3, bucketName string, key *string) ([]*s3.Tag, error) {
	tagInput := s3.GetObjectTaggingInput{
		Bucket: &bucketName,
		Key:    key,
	}
	tags, err := s3Client.GetObjectTaggingWithContext(ctx, &tagInput)
	if err != nil {
		// Try again
		fmt.Printf("error fetching tags for %v: %v (1/2)\n", *key, err)
		time.Sleep(10 * time.Millisecond)
		tags, err = s3Client.GetObjectTaggingWithContext(ctx, &tagInput)
		if err != nil {
			return nil, fmt.Errorf("error fetching tags for %v: %w (2/2)", *key, err)
		}
	}
	return tags.TagSet, nil
}

func fetchObjectsAndTags(ctx context.Context, s3Client *s3.S3, bucketName string, objects *s3.ListObjectsOutput) ([]*Object, error) {
	items := make([]*Object, 0)

	objectsChan := make(chan *s3.Object, 30)

	go func() {
		defer close(objectsChan)
		for _, o := range objects.Contents {
			o := o
			objectsChan <- o
		}
	}()

	m := sync.Mutex{}

	errGroup := errgroup.Group{}

	for o := range objectsChan {
		obj := o

		errGroup.Go(func() error {
			tags, err := fetchObjectTags(ctx, s3Client, bucketName, obj.Key)
			if err != nil {
				return err
			}

			object := NewObjectWithTags(obj, tags)

			m.Lock()
			items = append(items, object) //[index] = object
			m.Unlock()
			return nil
		})
	}

	err := errGroup.Wait()
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return items, err
	}

	return items, nil
}

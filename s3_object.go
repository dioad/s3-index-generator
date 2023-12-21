package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/cenkalti/backoff/v3"
	"golang.org/x/sync/semaphore"
)

type Object interface {
	Key() string
	LastModified() time.Time
	Size() int64
	BaseName() string
	Tags() map[string]string
	SetTags(tags map[string]string)
}

type ObjectTagSetter interface {
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

// FetchObjectsWithContext fetches objects from S3 with a context.
// func FetchObjectsWithContext(ctx context.Context, s3Client *s3.S3, bucketName string, prefix string) ([]Object, error) {
func FetchObjectsWithContext(ctx context.Context, bucket ObjectLister, prefix string) ([]Object, error) {
	return bucket.ListObjects(ctx, prefix)
}

type ObjectLister interface {
	ListObjects(ctx context.Context, prefix string) ([]Object, error)
}

type ObjectTagFetcher interface {
	FetchObjectTags(ctx context.Context, key string) (map[string]string, error)
}

type S3Bucket struct {
	s3Client             *s3.S3
	bucketName           string
	serverSideEncryption string
}

func NewS3Bucket(sess *session.Session, bucketName string, serverSideEncryption string) *S3Bucket {
	client := s3Client(sess)
	return &S3Bucket{
		s3Client:             client,
		bucketName:           bucketName,
		serverSideEncryption: serverSideEncryption,
	}
}

func (l *S3Bucket) ListObjects(ctx context.Context, prefix string) ([]Object, error) {
	objInput := s3.ListObjectsInput{
		Bucket: &l.bucketName,
		Prefix: &prefix,
	}

	items := make([]Object, 0)

	err := l.s3Client.ListObjectsPagesWithContext(ctx, &objInput, func(page *s3.ListObjectsOutput, lastPage bool) bool {
		localItems := make([]Object, len(page.Contents))
		for i, o := range page.Contents {
			localItems[i] = NewObject(o)
		}
		items = append(items, localItems...)

		return true
	})

	if err != nil {
		return items, fmt.Errorf("error listing objects: %v", err)
	}

	maxWorkers := runtime.GOMAXPROCS(0)
	sem := semaphore.NewWeighted(int64(maxWorkers))
	errorChannel := make(chan error, len(items))

	for index, o := range items {
		idx := index
		obj := o
		if err := sem.Acquire(ctx, 1); err != nil {
			errorChannel <- fmt.Errorf("error acquiring semaphore: %v", err)
			continue
		}

		go func() {
			defer sem.Release(1)

			tags, err := l.fetchObjectTags(ctx, obj.Key())
			if err != nil {
				errorChannel <- fmt.Errorf("error fetching tags for %v: %v", obj.Key(), err)
				return
			}

			items[idx].SetTags(tags)
		}()
	}

	close(errorChannel)

	if len(errorChannel) > 0 {
		return items, fmt.Errorf("there were one or more errors processing the objects")
	}

	return items, err
}

func (l *S3Bucket) fetchObjectTags(ctx context.Context, key string) (map[string]string, error) {
	tagInput := s3.GetObjectTaggingInput{
		Bucket: &l.bucketName,
		Key:    &key,
	}

	retryBackoff := backoff.NewExponentialBackOff()
	retryBackoff.MaxElapsedTime = 15 * time.Second

	var tags *s3.GetObjectTaggingOutput
	var err error

	err = backoff.Retry(func() error {
		tags, err = l.s3Client.GetObjectTaggingWithContext(ctx, &tagInput)
		return err
	}, retryBackoff)

	if err != nil {
		return nil, fmt.Errorf("error fetching tags for %v: %w", key, err)
	}

	tagMap := make(map[string]string)
	for _, tag := range tags.TagSet {
		tagMap[*tag.Key] = *tag.Value
	}

	return tagMap, nil
}

/*
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

func fetchObjectsAndTags(ctx context.Context, s3Client *s3.S3, bucketName string, objects *s3.ListObjectsOutput) ([]Object, error) {
	items := make([]Object, 0)

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
*/

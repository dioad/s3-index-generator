package main

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/cenkalti/backoff/v3"
	"golang.org/x/sync/errgroup"
)

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

func (l *S3Bucket) UpdateObjectsWithTags(ctx context.Context, items []Object) error {
	maxWorkers := runtime.GOMAXPROCS(0)

	eg := errgroup.Group{}
	eg.SetLimit(maxWorkers)

	for index, o := range items {
		idx := index
		obj := o

		eg.Go(func() error {
			tags, err := l.fetchObjectTags(ctx, obj.Key())

			if err != nil {
				return fmt.Errorf("error fetching tags for %v: %w", obj.Key(), err)
			}

			items[idx].SetTags(tags)

			return nil
		})
	}

	err := eg.Wait()

	return err
}

func (l *S3Bucket) ListObjectsWithTags(ctx context.Context, prefix string) ([]Object, error) {
	items, err := l.ListObjects(ctx, prefix)
	if err != nil {
		return items, fmt.Errorf("error listing objects: %w", err)
	}

	err = l.UpdateObjectsWithTags(ctx, items)

	return items, err
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

	return items, nil
}

func (l *S3Bucket) fetchObjectTags(ctx context.Context, key string) (map[string]string, error) {
	return fetchObjectTags(ctx, l.s3Client, l.bucketName, key)
}

func fetchObjectTags(ctx context.Context, client *s3.S3, bucketName string, key string) (map[string]string, error) {
	tagInput := s3.GetObjectTaggingInput{
		Bucket: &bucketName,
		Key:    &key,
	}

	retryBackoff := backoff.NewExponentialBackOff()
	retryBackoff.MaxElapsedTime = 15 * time.Second

	var tags *s3.GetObjectTaggingOutput
	var err error

	err = backoff.Retry(func() error {
		tags, err = client.GetObjectTaggingWithContext(ctx, &tagInput)
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

package main

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/cenkalti/backoff/v3"
	"golang.org/x/sync/errgroup"
)

// s3ObjectAPI is the subset of the S3 API used by S3Bucket, allowing mocking in tests.
type s3ObjectAPI interface {
	ListObjectsPagesWithContext(ctx context.Context, input *s3.ListObjectsInput, fn func(*s3.ListObjectsOutput, bool) bool, opts ...request.Option) error
	GetObjectTaggingWithContext(ctx context.Context, input *s3.GetObjectTaggingInput, opts ...request.Option) (*s3.GetObjectTaggingOutput, error)
}

type S3Bucket struct {
	s3Client             s3ObjectAPI
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
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 15 * time.Second
	return fetchObjectTagsWithBackoff(ctx, l.s3Client, l.bucketName, key, b)
}

// fetchObjectTagsWithBackoff fetches S3 object tags, retrying according to the provided backoff strategy.
// Separating the backoff allows tests to inject a fast/no-wait strategy.
func fetchObjectTagsWithBackoff(ctx context.Context, client s3ObjectAPI, bucketName string, key string, b backoff.BackOff) (map[string]string, error) {
	tagInput := s3.GetObjectTaggingInput{
		Bucket: &bucketName,
		Key:    &key,
	}

	var tags *s3.GetObjectTaggingOutput
	var err error

	err = backoff.Retry(func() error {
		tags, err = client.GetObjectTaggingWithContext(ctx, &tagInput)
		return err
	}, b)

	if err != nil {
		return nil, fmt.Errorf("error fetching tags for %v: %w", key, err)
	}

	tagMap := make(map[string]string)
	for _, tag := range tags.TagSet {
		tagMap[*tag.Key] = *tag.Value
	}

	return tagMap, nil
}

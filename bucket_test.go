package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/cenkalti/backoff/v3"
)

// mockS3Client implements s3ObjectAPI for testing.
type mockS3Client struct {
	listObjectsPages func(ctx context.Context, input *s3.ListObjectsInput, fn func(*s3.ListObjectsOutput, bool) bool) error
	getObjectTagging func(ctx context.Context, input *s3.GetObjectTaggingInput) (*s3.GetObjectTaggingOutput, error)
}

func (m *mockS3Client) ListObjectsPagesWithContext(ctx context.Context, input *s3.ListObjectsInput, fn func(*s3.ListObjectsOutput, bool) bool, opts ...request.Option) error {
	return m.listObjectsPages(ctx, input, fn)
}

func (m *mockS3Client) GetObjectTaggingWithContext(ctx context.Context, input *s3.GetObjectTaggingInput, opts ...request.Option) (*s3.GetObjectTaggingOutput, error) {
	return m.getObjectTagging(ctx, input)
}

func newBucketWithMock(mock s3ObjectAPI) *S3Bucket {
	return &S3Bucket{s3Client: mock, bucketName: "test-bucket"}
}

func TestListObjects_SinglePage(t *testing.T) {
	mock := &mockS3Client{
		listObjectsPages: func(_ context.Context, _ *s3.ListObjectsInput, fn func(*s3.ListObjectsOutput, bool) bool) error {
			fn(&s3.ListObjectsOutput{
				Contents: []*s3.Object{
					{Key: aws.String("a/b.zip")},
					{Key: aws.String("a/c.zip")},
				},
			}, true)
			return nil
		},
	}
	bucket := newBucketWithMock(mock)

	objects, err := bucket.ListObjects(context.Background(), "a/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objects) != 2 {
		t.Errorf("expected 2 objects, got %d", len(objects))
	}
}

func TestListObjects_MultiplePages(t *testing.T) {
	page1 := []*s3.Object{{Key: aws.String("page1/obj1")}, {Key: aws.String("page1/obj2")}}
	page2 := []*s3.Object{{Key: aws.String("page2/obj1")}}

	calls := 0
	mock := &mockS3Client{
		listObjectsPages: func(_ context.Context, _ *s3.ListObjectsInput, fn func(*s3.ListObjectsOutput, bool) bool) error {
			calls++
			fn(&s3.ListObjectsOutput{Contents: page1}, false)
			fn(&s3.ListObjectsOutput{Contents: page2}, true)
			return nil
		},
	}
	bucket := newBucketWithMock(mock)

	objects, err := bucket.ListObjects(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objects) != 3 {
		t.Errorf("expected 3 objects across 2 pages, got %d", len(objects))
	}
}

func TestListObjects_Error(t *testing.T) {
	mock := &mockS3Client{
		listObjectsPages: func(_ context.Context, _ *s3.ListObjectsInput, _ func(*s3.ListObjectsOutput, bool) bool) error {
			return fmt.Errorf("s3 unavailable")
		},
	}
	bucket := newBucketWithMock(mock)

	_, err := bucket.ListObjects(context.Background(), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFetchObjectTagsWithBackoff_Success(t *testing.T) {
	mock := &mockS3Client{
		getObjectTagging: func(_ context.Context, _ *s3.GetObjectTaggingInput) (*s3.GetObjectTaggingOutput, error) {
			return &s3.GetObjectTaggingOutput{
				TagSet: []*s3.Tag{
					{Key: aws.String("env"), Value: aws.String("prod")},
					{Key: aws.String("team"), Value: aws.String("platform")},
				},
			}, nil
		},
	}

	tags, err := fetchObjectTagsWithBackoff(context.Background(), mock, "test-bucket", "some/key.zip", &backoff.StopBackOff{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tags["env"] != "prod" || tags["team"] != "platform" {
		t.Errorf("unexpected tags: %v", tags)
	}
}

func TestFetchObjectTagsWithBackoff_Error(t *testing.T) {
	mock := &mockS3Client{
		getObjectTagging: func(_ context.Context, _ *s3.GetObjectTaggingInput) (*s3.GetObjectTaggingOutput, error) {
			return nil, fmt.Errorf("access denied")
		},
	}

	// StopBackOff tries once and stops immediately.
	_, err := fetchObjectTagsWithBackoff(context.Background(), mock, "test-bucket", "some/key.zip", &backoff.StopBackOff{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFetchObjectTagsWithBackoff_RetryThenSuccess(t *testing.T) {
	attempts := 0
	mock := &mockS3Client{
		getObjectTagging: func(_ context.Context, _ *s3.GetObjectTaggingInput) (*s3.GetObjectTaggingOutput, error) {
			attempts++
			if attempts < 3 {
				return nil, fmt.Errorf("transient error")
			}
			return &s3.GetObjectTaggingOutput{
				TagSet: []*s3.Tag{{Key: aws.String("k"), Value: aws.String("v")}},
			}, nil
		},
	}

	// ZeroBackOff retries immediately with no wait — safe in tests.
	tags, err := fetchObjectTagsWithBackoff(context.Background(), mock, "test-bucket", "some/key.zip", backoff.WithMaxRetries(&backoff.ZeroBackOff{}, 5))
	if err != nil {
		t.Fatalf("unexpected error after retries: %v", err)
	}
	if tags["k"] != "v" {
		t.Errorf("expected tag k=v, got %v", tags)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestUpdateObjectsWithTags(t *testing.T) {
	mock := &mockS3Client{
		getObjectTagging: func(_ context.Context, input *s3.GetObjectTaggingInput) (*s3.GetObjectTaggingOutput, error) {
			return &s3.GetObjectTaggingOutput{
				TagSet: []*s3.Tag{{Key: aws.String("tagged"), Value: aws.String(*input.Key)}},
			}, nil
		},
	}
	bucket := newBucketWithMock(mock)

	objects := []Object{
		simpleObject("product/1.0.0/artifact.zip"),
		simpleObject("product/1.0.0/artifact2.zip"),
	}

	err := bucket.UpdateObjectsWithTags(context.Background(), objects)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, o := range objects {
		if o.Tags()["tagged"] != o.Key() {
			t.Errorf("expected tag 'tagged' = %q, got %q", o.Key(), o.Tags()["tagged"])
		}
	}
}

func TestListObjectsWithTags(t *testing.T) {
	mock := &mockS3Client{
		listObjectsPages: func(_ context.Context, _ *s3.ListObjectsInput, fn func(*s3.ListObjectsOutput, bool) bool) error {
			fn(&s3.ListObjectsOutput{
				Contents: []*s3.Object{{Key: aws.String("a/b.zip")}},
			}, true)
			return nil
		},
		getObjectTagging: func(_ context.Context, _ *s3.GetObjectTaggingInput) (*s3.GetObjectTaggingOutput, error) {
			return &s3.GetObjectTaggingOutput{
				TagSet: []*s3.Tag{{Key: aws.String("version"), Value: aws.String("1.0")}},
			}, nil
		},
	}
	bucket := newBucketWithMock(mock)

	objects, err := bucket.ListObjectsWithTags(context.Background(), "a/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objects))
	}
	if objects[0].Tags()["version"] != "1.0" {
		t.Errorf("expected tag version=1.0, got %v", objects[0].Tags())
	}
}

package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/cenkalti/backoff/v3"
)

// mockS3Client implements s3ObjectAPI for testing.
type mockS3Client struct {
	listObjectsV2    func(ctx context.Context, input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error)
	getObjectTagging func(ctx context.Context, input *s3.GetObjectTaggingInput) (*s3.GetObjectTaggingOutput, error)
}

func (m *mockS3Client) ListObjectsV2(ctx context.Context, input *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	return m.listObjectsV2(ctx, input)
}

func (m *mockS3Client) GetObjectTagging(ctx context.Context, input *s3.GetObjectTaggingInput, _ ...func(*s3.Options)) (*s3.GetObjectTaggingOutput, error) {
	return m.getObjectTagging(ctx, input)
}

func newBucketWithMock(mock s3ObjectAPI) *S3Bucket {
	return &S3Bucket{s3Client: mock, bucketName: "test-bucket"}
}

func TestListObjects_SinglePage(t *testing.T) {
	mock := &mockS3Client{
		listObjectsV2: func(_ context.Context, _ *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("a/b.zip")},
					{Key: aws.String("a/c.zip")},
				},
				IsTruncated: aws.Bool(false),
			}, nil
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
	page1 := []s3types.Object{{Key: aws.String("page1/obj1")}, {Key: aws.String("page1/obj2")}}
	page2 := []s3types.Object{{Key: aws.String("page2/obj1")}}

	calls := 0
	mock := &mockS3Client{
		listObjectsV2: func(_ context.Context, input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
			calls++
			if input.ContinuationToken == nil {
				token := "token1"
				return &s3.ListObjectsV2Output{
					Contents:              page1,
					IsTruncated:           aws.Bool(true),
					NextContinuationToken: &token,
				}, nil
			}
			return &s3.ListObjectsV2Output{
				Contents:    page2,
				IsTruncated: aws.Bool(false),
			}, nil
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
	if calls != 2 {
		t.Errorf("expected 2 API calls for 2 pages, got %d", calls)
	}
}

func TestListObjects_Error(t *testing.T) {
	mock := &mockS3Client{
		listObjectsV2: func(_ context.Context, _ *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
			return nil, fmt.Errorf("s3 unavailable")
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
				TagSet: []s3types.Tag{
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
				TagSet: []s3types.Tag{{Key: aws.String("k"), Value: aws.String("v")}},
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
				TagSet: []s3types.Tag{{Key: aws.String("tagged"), Value: input.Key}},
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
		listObjectsV2: func(_ context.Context, _ *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents:    []s3types.Object{{Key: aws.String("a/b.zip")}},
				IsTruncated: aws.Bool(false),
			}, nil
		},
		getObjectTagging: func(_ context.Context, _ *s3.GetObjectTaggingInput) (*s3.GetObjectTaggingOutput, error) {
			return &s3.GetObjectTaggingOutput{
				TagSet: []s3types.Tag{{Key: aws.String("version"), Value: aws.String("1.0")}},
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

package main

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestNewObject(t *testing.T) {
	o := simpleObject("testKey")
	if o.Key() != "testKey" {
		t.Errorf("NewObject() = %v, want %v", o.Key(), "testKey")
	}
}

func TestNewObjectWithTags(t *testing.T) {
	o := simpleObject("testKey")
	o.SetTags(map[string]string{
		"tagKey": "tagValue",
	})

	if o.Key() != "testKey" || o.Tags()["tagKey"] != "tagValue" {
		t.Errorf("NewObjectWithTags() failed, object not created correctly")
	}
}

func TestObjectMethods(t *testing.T) {
	now := time.Now()
	obj := s3types.Object{
		Key:          aws.String("testKey"),
		LastModified: &now,
		Size:         aws.Int64(100),
	}
	o := NewObject(obj)
	if o.Key() != "testKey" {
		t.Errorf("Key() = %v, want %v", o.Key(), "testKey")
	}
	if o.LastModified().IsZero() {
		t.Errorf("LastModified() returned zero value")
	}
	if o.Size() != 100 {
		t.Errorf("Size() = %v, want %v", o.Size(), 100)
	}
	if o.BaseName() != "testKey" {
		t.Errorf("BaseName() = %v, want %v", o.BaseName(), "testKey")
	}
}

func TestIndexEntry(t *testing.T) {
	o := simpleObject("data/TestProduct/1.0.0/TestProduct_linux_amd64.zip")
	cfg := IndexConfig{
		KeyExtractions: ReleaseDetailKeyExtractions{
			DefaultReleaseInfoKeyExtractor,
		},
	}
	entry, err := NewIndexEntry(cfg, o)
	if err != nil {
		t.Fatalf("NewIndexEntry() failed, %v", err)
	}

	if entry.Name != "TestProduct" || entry.Version != "1.0.0" {
		t.Errorf("IndexEntry() failed, entry fields not correctly set")
	}
}

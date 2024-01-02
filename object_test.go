package main

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
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
	obj := &s3.Object{
		Key:          stringToPointer("testKey"),
		LastModified: timeToPointer(time.Now()),
		Size:         int64ToPointer(100),
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

// Helper functions to create pointers to string, time.Time, and int64
func stringToPointer(s string) *string {
	return &s
}

func timeToPointer(t time.Time) *time.Time {
	return &t
}

func int64ToPointer(i int64) *int64 {
	return &i
}

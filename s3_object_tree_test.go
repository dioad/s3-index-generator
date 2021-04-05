package main

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/s3"
)

func TestObjectTree(t *testing.T) {
	var fileA = "a/b/c/fileA"
	var fileB = "a/b/c/fileB"
	var fileC = "a/b/fileC"

	stubs := []*s3.Object{
		&s3.Object{
			Key: &fileA,
		},
		&s3.Object{
			Key: &fileB,
		},
		&s3.Object{
			Key: &fileC,
		},
	}

	tree := CreateObjectTree(stubs)

	aChildLen := len(tree.Children["a"].Children)
	if aChildLen != 1 {
		t.Fatalf("exptect children of `a` length 1, got %v", aChildLen)
	}

	bTree := tree.Children["a"].Children["b"]

	bChildLen := len(bTree.Children)
	if bChildLen != 1 {
		t.Fatalf("exptect children of `a` length 1, got %v", bChildLen)
	}

	bObjLen := len(bTree.Objects)
	if bObjLen != 1 {
		t.Fatalf("exptect children of `a` length 1, got %v", bObjLen)
	}
}

func TestExclusions(t *testing.T) {
	tests := map[string]struct {
		objectKey  string
		exclusions Exclusions
		include    bool
	}{
		"key exclude blah.zip": {
			objectKey:  "blah.zip",
			exclusions: Exclusions{ExcludeKey("blah.zip")},
			include:    false,
		},
		"suffix exclude index.html": {
			objectKey:  "index.html",
			exclusions: Exclusions{ExcludeSuffix("index.html")},
			include:    false,
		},
		"suffix include blah.zip": {
			objectKey:  "blah.zip",
			exclusions: Exclusions{ExcludeSuffix("index.html")},
			include:    true,
		},
		"prefix exclude .asdf/ prefix": {
			objectKey:  ".asdf/blah",
			exclusions: Exclusions{ExcludePrefix(".")},
			include:    false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			include := tc.exclusions.Include(tc.objectKey)
			if include != tc.include {
				t.Fatalf("got %v, expected %v", include, tc.include)
			}
		})
	}
}

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
		t.Fatalf("exptect children of `a` length 1, got %v", aChildLen )
	}

	bTree := tree.Children["a"].Children["b"]

	bChildLen := len(bTree.Children)
	if bChildLen != 1 {
		t.Fatalf("exptect children of `a` length 1, got %v", bChildLen )
	}

	bObjLen := len(bTree.Objects)
	if bObjLen != 1 {
		t.Fatalf("exptect children of `a` length 1, got %v", bObjLen )
	}
}
package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go/service/s3"
)

func TestObjectTree(t *testing.T) {
	var fileA = "a/b/c/fileA"
	var fileB = "a/b/c/fileB"
	var fileC = "a/b/fileC"

	stubs := []Object{
		simpleObject(fileA),
		simpleObject(fileB),
		simpleObject(fileC),
	}

	tree := NewObjectTreeWithObjects(ObjectTreeConfig{}, stubs)

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
			exclusions: Exclusions{HasKey("blah.zip")},
			include:    false,
		},
		"suffix exclude index.html": {
			objectKey:  "index.html",
			exclusions: Exclusions{HasSuffix("index.html")},
			include:    false,
		},
		"suffix include blah.zip": {
			objectKey:  "blah.zip",
			exclusions: Exclusions{HasSuffix("index.html")},
			include:    true,
		},
		"prefix exclude .asdf/ prefix": {
			objectKey:  ".asdf/blah",
			exclusions: Exclusions{HasPrefix(".")},
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

func TestAddObject(t *testing.T) {
	tree := &ObjectTree{}
	obj := &object{obj: &s3.Object{Key: stringToPointer("testKey")}}
	tree.AddObject(obj)
	if len(tree.Objects) != 1 {
		t.Errorf("AddObject() failed, object not added")
	}
}

func TestAddChild(t *testing.T) {
	tree := &ObjectTree{}
	tree.AddChild("testChild")
	if _, ok := tree.Children["testChild"]; !ok {
		t.Errorf("AddChild() failed, child not added")
	}
}

func TestProductTree(t *testing.T) {
	var fileA = "product/1.2.0/v1.2.3/product_linux_amd64.zip"

	stubs := []Object{
		simpleObject(fileA),
	}

	tree := NewObjectTreeWithObjects(ObjectTreeConfig{}, stubs)

	if !IsArchiveTree(tree) {
		t.Fatalf("expected product tree, got %v", tree)
	}
}

func TestWalker(t *testing.T) {
	var fileA = "product/1.2.0/v1.2.3/product_linux_amd64.zip"

	stubs := []Object{
		simpleObject(fileA),
	}

	tree := NewObjectTreeWithObjects(ObjectTreeConfig{}, stubs)

	paths := make([]Object, 0)

	walker := func(tree *ObjectTree) error {
		fmt.Printf("walker: %v\n", filepath.Join(tree.FullPath, "index.json"))

		paths = append(paths, tree.Objects...)
		return nil
	}

	err := tree.Walk(walker, true, true)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	fmt.Printf("%v\n", paths)
}

// Continue with the rest of the tests...

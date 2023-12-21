package main

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/s3"
)

func TestObjectTree(t *testing.T) {
	var fileA = "a/b/c/fileA"
	var fileB = "a/b/c/fileB"
	var fileC = "a/b/fileC"

	stubs := []Object{
		&object{
			obj: &s3.Object{
				Key: &fileA,
			},
		},
		&object{
			obj: &s3.Object{
				Key: &fileB,
			},
		},
		&object{
			obj: &s3.Object{
				Key: &fileC,
			},
		},
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

func TestAddExclusion(t *testing.T) {
	tree := &ObjectTree{}
	excludeFunc := ExcludeKey("testKey")
	tree.AddExclusion(excludeFunc)
	if len(tree.Exclusions) != 1 {
		t.Errorf("AddExclusion() failed, exclusion not added")
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

func TestIsVersionTree(t *testing.T) {
	tree := &ObjectTree{DirName: "build"}
	if !IsVersionTree(tree) {
		t.Errorf("IsVersionTree() = false, want true")
	}
}

func TestIsArchiveTree(t *testing.T) {
	tree := &ObjectTree{}
	child := tree.AddChild("testProduct")
	child.AddChild("1.0.0")
	if !IsArchiveTree(tree) {
		t.Errorf("IsArchiveTree() = false, want true")
	}
}

func TestIsProductTree(t *testing.T) {
	tree := &ObjectTree{}
	tree.AddChild("build")

	if !IsProductTree(tree) {
		t.Errorf("IsProductTree() = false, want true")
	}
}

// Continue with the rest of the tests...

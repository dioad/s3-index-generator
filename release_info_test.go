package main

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

func TestIsVersionTreeWithBuildPath(t *testing.T) {
	tree := &ObjectTree{DirName: "build"}
	if !IsVersionTree(tree) {
		t.Errorf("IsVersionTree() = false, want true")
	}
}

func TestIsVersionTreeWithVersionPath(t *testing.T) {
	tree := &ObjectTree{DirName: "2.3.4"}
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

func TestIsProductTreeWithBuildPath(t *testing.T) {
	tree := &ObjectTree{}
	tree.AddChild("build")

	if !IsProductTree(tree) {
		t.Errorf("IsProductTree() = false, want true")
	}
}

func TestIsProductTreeWithVersionPath(t *testing.T) {
	tree := &ObjectTree{}
	tree.AddChild("1.2.3")

	if !IsProductTree(tree) {
		t.Errorf("IsProductTree() = false, want true")
	}
}

func TestIsProductTreeWithInvalidPath(t *testing.T) {
	tree := &ObjectTree{}
	tree.AddChild("invalid")

	if IsProductTree(tree) {
		t.Errorf("IsProductTree() = true, want false")
	}
}

func TestNewArchiveIndexForObjectTree(t *testing.T) {
	tree := &ObjectTree{}
	tree.AddObject(simpleObject("testProduct/1.0.0/testProduct_linux_amd64.zip"))

	cfg := IndexConfig{}
	index := NewArchiveIndexForObjectTree(cfg, tree)
	if index == nil {
		t.Errorf("NewArchiveIndexForObjectTree() = nil, want not nil")
	}
}

func simpleObject(key string) Object {
	return &object{obj: &s3.Object{Key: aws.String(key)}}
}

func TestNewProductIndexForObjectTree(t *testing.T) {
	tree := &ObjectTree{}
	tree.AddObject(simpleObject("1.0.0/testProduct_linux_amd64.zip"))

	cfg := IndexConfig{}
	index := NewProductIndexForObjectTree(cfg, tree)
	if index == nil {
		t.Errorf("NewProductIndexForObjectTree() = nil, want not nil")
	}
}

func TestNewVersionIndexForObjectTree(t *testing.T) {
	tree := &ObjectTree{
		DirName: "1.0.0",
	}
	tree.AddObject(simpleObject("testProduct_linux_amd64.zip"))

	//child.AddChild("1.0.0")
	cfg := IndexConfig{}
	index := NewVersionIndexForObjectTree(cfg, tree)
	if index == nil {
		t.Errorf("NewVersionIndexForObjectTree() = nil, want not nil")
	}
}

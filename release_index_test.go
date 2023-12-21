package main

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/s3"
)

func TestNewArchiveIndex(t *testing.T) {
	index := NewArchiveIndex()
	if len(index.Product) != 0 {
		t.Errorf("NewArchiveIndex() = %v, want %v", len(index.Product), 0)
	}
}

func TestAddProduct(t *testing.T) {
	index := NewArchiveIndex()
	product := NewProductIndex("TestProduct")
	index.AddProduct(product)
	if _, ok := index.Product["TestProduct"]; !ok {
		t.Errorf("AddProduct() failed, product not added")
	}
}

func TestNewProductIndex(t *testing.T) {
	product := NewProductIndex("TestProduct")
	if product.Name != "TestProduct" {
		t.Errorf("NewProductIndex() = %v, want %v", product.Name, "TestProduct")
	}
	if len(product.Versions) != 0 {
		t.Errorf("NewProductIndex() = %v, want %v", len(product.Versions), 0)
	}
}

func TestAddVersion(t *testing.T) {
	product := NewProductIndex("TestProduct")
	version := &VersionIndex{Version: "1.0.0"}
	product.AddVersion(version)
	if _, ok := product.Versions["1.0.0"]; !ok {
		t.Errorf("AddVersion() failed, version not added")
	}
	if product.LatestVersion != version {
		t.Errorf("AddVersion() failed, LatestVersion not updated")
	}
}

func TestNewIndexEntry(t *testing.T) {
	cfg := IndexConfig{
		ArchitectureTagName: "Dioad/Arch",
		ProductTagName:      "Dioad/Project",
		OSTagName:           "Dioad/OS",
		VersionTagName:      "Dioad/Version",
	}
	obj := &object{
		tags: map[string]string{
			"Dioad/Arch":    "x86",
			"Dioad/Project": "TestProject",
			"Dioad/OS":      "linux",
			"Dioad/Version": "1.0.0",
		},
	}
	entry := NewIndexEntry(cfg, obj)
	if entry.Arch != "x86" || entry.Name != "TestProject" || entry.Os != "linux" || entry.Version != "1.0.0" {
		t.Errorf("NewIndexEntry() failed, entry fields not correctly set")
	}
}

func TestNewVersionIndex(t *testing.T) {
	cfg := IndexConfig{
		ArchitectureTagName: "Dioad/Arch",
		ProductTagName:      "Dioad/Project",
		OSTagName:           "Dioad/OS",
		VersionTagName:      "Dioad/Version",
	}
	obj := &object{
		obj: &s3.Object{
			Key: stringToPointer("testKey"),
		},
		tags: map[string]string{
			"Dioad/Arch":    "x86",
			"Dioad/Project": "TestProject",
			"Dioad/OS":      "linux",
			"Dioad/Version": "1.0.0",
		},
	}
	rootTree := NewRootObjectTree(ObjectTreeConfig{PrefixToStrip: "TestProject"})
	productTree := rootTree.AddChild("TestProject")
	versionTree := productTree.AddChild("1.0.0")
	versionTree.AddObject(obj)
	//tree := &ObjectTree{
	//	DirName: "1.0.0",
	//	Objects: []Object{obj},
	//}
	versionIndex := NewVersionIndex(cfg, versionTree)
	if versionIndex.Name == "" || versionIndex.Version != "1.0.0" || len(versionIndex.Builds) != 1 {
		t.Errorf("NewVersionIndex() failed, versionIndex fields not correctly set")
	}
}

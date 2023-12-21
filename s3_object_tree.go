package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// ObjectTree is a tree of S3 objects.
type ObjectTree struct {
	Title         string // extract to Page
	Nonce         string // extract to Page
	FullPath      string
	DirName       string
	Objects       []Object
	Children      map[string]*ObjectTree
	Exclusions    Exclusions
	PrefixToStrip string
}

// ExcludeFunc is a function that excludes paths.
type ExcludeFunc func(string) bool

// Exclusions is a list of functions that exclude paths.
type Exclusions []ExcludeFunc

// Include returns true if the path should be included.
func (e Exclusions) Include(key string) bool {
	// log.Printf("Include: %v", key)
	for _, excludeFunc := range e {
		if excludeFunc(key) {
			return false
		}
	}
	return true
}

// ExcludeKey returns a function that excludes paths with the given key.
func ExcludeKey(key string) ExcludeFunc {
	return func(path string) bool {
		//log.Printf("ExcludeKey: comparing '%v' with '%v'", key, path)
		//log.Printf("  returning %v", key == path)
		return key == path
	}
}

// ExcludePrefix returns a function that excludes paths with the given prefix.
func ExcludePrefix(prefix string) ExcludeFunc {
	return func(path string) bool {
		//log.Printf("ExcludePrefix: comparing '%v' with '%v'", prefix, path)
		//log.Printf("  returning %v", strings.HasPrefix(path, prefix))
		return strings.HasPrefix(path, prefix)
	}
}

// ExcludeSuffix returns a function that excludes paths with the given suffix.
func ExcludeSuffix(suffix string) ExcludeFunc {
	return func(path string) bool {
		//log.Printf("ExcludeSuffix: comparing '%v' with '%v'", suffix, path)
		//log.Printf("  returning %v", strings.HasSuffix(path, suffix))
		return strings.HasSuffix(path, suffix)
	}
}

// AddExclusion adds an exclusion to the tree.
func (t *ObjectTree) AddExclusion(f ExcludeFunc) {
	if t.Exclusions == nil {
		t.Exclusions = make(Exclusions, 0)
	}
	t.Exclusions = append(t.Exclusions, f)
}

// AddChild adds a child to the tree, if it doesn't already exist.
func (t *ObjectTree) AddChild(name string) *ObjectTree {
	if t.Children == nil {
		t.Children = make(map[string]*ObjectTree)
	}

	if !t.Exclusions.Include(name) {
		return nil
	}

	if _, exists := t.Children[name]; !exists {
		t.Children[name] = &ObjectTree{
			DirName:    name,
			FullPath:   filepath.Join(t.FullPath, name),
			Exclusions: t.Exclusions,
		}
	}

	return t.Children[name]
}

func IsVersionTree(t *ObjectTree) bool {
	return t.DirName == "build" || IsVersionLabel(t.DirName)
}

func IsArchiveTree(t *ObjectTree) bool {
	for _, v := range t.Children {
		if IsProductTree(v) {
			return true
		}
	}
	return false
}

func IsProductTree(t *ObjectTree) bool {
	for _, v := range t.Children {
		if IsVersionTree(v) {
			return true
		}
	}
	return false
}

func NewArchiveIndexForObjectTree(cfg IndexConfig, t *ObjectTree) *ArchiveIndex {
	if !IsArchiveTree(t) {
		return nil
	}

	archiveIndex := NewArchiveIndex()

	for _, v := range t.Children {
		if IsProductTree(v) {
			productIndex := NewProductIndexForObjectTree(cfg, v)
			archiveIndex.AddProduct(productIndex)
		}
	}
	return archiveIndex
}

func NewProductIndexForObjectTree(cfg IndexConfig, t *ObjectTree) *ProductIndex {
	if !IsProductTree(t) {
		return nil
	}
	productIndex := NewProductIndex(t.DirName)

	for _, v := range t.Children {
		if IsVersionTree(v) {
			productIndex.AddVersion(v.VersionIndex(cfg))
		}
	}

	return productIndex
}

func NewVersionIndexForObjectTree(cfg IndexConfig, t *ObjectTree) *VersionIndex {
	if !IsVersionTree(t) {
		return nil
	}

	versionIndex := NewVersionIndex(cfg, t)

	return versionIndex
}

// Deprecated: Use ProductIndex instead.
func (t *ObjectTree) ProductIndex(cfg IndexConfig) *ProductIndex {
	return NewProductIndexForObjectTree(cfg, t)
}

func (t *ObjectTree) ParentName() string {
	return filepath.Base(t.ParentFullPath())
}

func (t *ObjectTree) ParentFullPath() string {
	return filepath.Clean(filepath.Join(t.FullPath, ".."))
}

// Deprecated: Use VersionIndex instead.
func (t *ObjectTree) VersionIndex(cfg IndexConfig) *VersionIndex {
	return NewVersionIndexForObjectTree(cfg, t)
}

// AddPathToTree adds a path to the tree.
//func AddPathToTree(t *ObjectTree, pathParts []string, obj Object) {
//	if len(pathParts) == 1 {
//		t.addSinglePartObject(obj)
//	} else {
//		newTree := t.AddChild(pathParts[0])
//		if newTree != nil {
//			AddPathToTree(newTree, pathParts[1:], obj)
//		}
//	}
//}

func (t *ObjectTree) addPathToTree(pathParts []string, obj Object) {
	if len(pathParts) == 1 {
		t.addSinglePartObject(obj)
	} else {
		newTree := t.AddChild(pathParts[0])
		if newTree != nil {
			newTree.addPathToTree(pathParts[1:], obj)
		}
	}
}

// AddObject adds an object to the tree, if it doesn't already exist.
func (t *ObjectTree) addSinglePartObject(obj Object) {
	if t.Objects == nil {
		t.Objects = make([]Object, 0)
	}

	//log.Printf("AddObject: %v", *obj.Key)
	if t.Exclusions.Include(obj.Key()) {
		//log.Printf("AddObject:Include: %v", *obj.Key)
		t.Objects = append(t.Objects, obj)
	}
}

func (t *ObjectTree) AddObject(obj Object) {
	parts := strings.Split(obj.Key(), "/")
	if len(parts) == 1 {
		t.addSinglePartObject(obj)
	} else {
		if parts[0] == t.PrefixToStrip {
			parts = parts[1:]
		}
		t.addPathToTree(parts, obj)
	}
}

func (t *ObjectTree) AddObjects(objects []Object) {
	for _, o := range objects {
		if o != nil {
			t.AddObject(o)
		}
	}
}

func (r *ObjectTree) AddObjectsFromLister(bucket ObjectLister) error {
	ctx := context.Background()

	objects, err := bucket.ListObjects(ctx, r.PrefixToStrip)
	if err != nil {
		return fmt.Errorf("error listing objects: %w", err)
	}

	r.AddObjects(objects)

	return nil
}

//// AddObjectToTree adds an object to the tree.
//func AddObjectToTree(t *ObjectTree, obj Object) {
//	for _, o := range objects {
//		AddObjectToTree(t, o)
//	}
//}
//
//// AddObjectsToTree adds objects to the tree.
//func AddObjectsToTree(t *ObjectTree, objects []Object) {
//	for _, o := range objects {
//		AddObjectToTree(t, o)
//	}
//}

// NewRootObjectTree creates a new root object tree.
func NewRootObjectTree(cfg ObjectTreeConfig) *ObjectTree {
	return &ObjectTree{
		FullPath:      "/",
		DirName:       "/",
		PrefixToStrip: cfg.PrefixToStrip,
		Exclusions:    cfg.Exclusions,
	}
}

func NewObjectTreeWithObjects(cfg ObjectTreeConfig, objects []Object) *ObjectTree {
	t := NewRootObjectTree(cfg)

	t.AddObjects(objects)

	return t
}

func NewObjectTreeFromLister(cfg ObjectTreeConfig, objectLister ObjectLister) (*ObjectTree, error) {
	t := NewRootObjectTree(cfg)

	err := t.AddObjectsFromLister(objectLister)
	if err != nil {
		return nil, err
	}

	return t, nil
}

// CreateObjectTree creates an object tree.
//func CreateObjectTree(objects []Object) *ObjectTree {
//	t := NewRootObjectTree()
//
//	t.AddObjects(objects)
//
//	return t
//}

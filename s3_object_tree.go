package main

import (
	"path/filepath"
	"strings"
)

// ObjectTree is a tree of S3 objects.
type ObjectTree struct {
	Title         string // extract to Page
	Nonce         string // extract to Page
	FullPath      string
	DirName       string
	Objects       []*Object
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

// AddObject adds an object to the tree, if it doesn't already exist.
func (t *ObjectTree) AddObject(obj *Object) {
	if t.Objects == nil {
		t.Objects = make([]*Object, 0)
	}

	//log.Printf("AddObject: %v", *obj.Key)
	if t.Exclusions.Include(obj.Key()) {
		//log.Printf("AddObject:Include: %v", *obj.Key)
		t.Objects = append(t.Objects, obj)
	}
}

// AddChild adds a child to the tree, if it doesn't already exist.
func (t *ObjectTree) AddChild(name string) *ObjectTree {
	if t.Children == nil {
		t.Children = make(map[string]*ObjectTree)
	}

	//log.Printf("AddChild:Include: %v", name)
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

// IsVersionTree returns true if the tree name looks like a semver.
func (t *ObjectTree) IsVersionTree() bool {
	return t.DirName == "build" || IsVersionLabel(t.DirName)
}

func (t *ObjectTree) IsArchiveTree() bool {
	for _, v := range t.Children {
		if v.IsProductTree() {
			return true
		}
	}
	return false
}

func (t *ObjectTree) ArchiveIndex(cfg IndexConfig) *ArchiveIndex {
	if !t.IsArchiveTree() {
		return nil
	}

	archiveIndex := NewArchiveIndex()

	for _, v := range t.Children {
		if v.IsProductTree() {
			archiveIndex.AddProduct(v.ProductIndex(cfg))
		}
	}
	return archiveIndex
}

// IsProductTree returns true if the tree contains children that look like a semver.
func (t *ObjectTree) IsProductTree() bool {
	for _, v := range t.Children {
		if v.IsVersionTree() {
			return true
		}
	}
	return false
}

func (t *ObjectTree) ProductIndex(cfg IndexConfig) *ProductIndex {
	if !t.IsProductTree() {
		return nil
	}

	productIndex := NewProductIndex(t.DirName)

	for _, v := range t.Children {
		if v.IsVersionTree() {
			productIndex.AddVersion(v.VersionIndex(cfg))
		}
	}

	return productIndex
}

func (t *ObjectTree) ParentName() string {
	return filepath.Base(t.ParentFullPath())
}

func (t *ObjectTree) ParentFullPath() string {
	return filepath.Clean(filepath.Join(t.FullPath, ".."))
}

func (t *ObjectTree) VersionIndex(cfg IndexConfig) *VersionIndex {
	if !t.IsVersionTree() {
		return nil
	}

	versionIndex := NewVersionIndex(cfg, t)

	return versionIndex
}

// AddPathToTree adds a path to the tree.
func AddPathToTree(t *ObjectTree, pathParts []string, obj *Object) {
	if len(pathParts) == 1 {
		t.AddObject(obj)
	} else {
		newTree := t.AddChild(pathParts[0])
		if newTree != nil {
			AddPathToTree(newTree, pathParts[1:], obj)
		}
	}
}

// AddObjectToTree adds an object to the tree.
func AddObjectToTree(t *ObjectTree, obj *Object) {
	parts := strings.Split(obj.Key(), "/")
	if len(parts) == 1 {
		t.AddObject(obj)
	} else {
		if parts[0] == t.PrefixToStrip {
			parts = parts[1:]
		}
		AddPathToTree(t, parts, obj)
	}
}

// AddObjectsToTree adds objects to the tree.
func AddObjectsToTree(t *ObjectTree, objects []*Object) {
	for _, o := range objects {
		AddObjectToTree(t, o)
	}
}

// NewRootObjectTree creates a new root object tree.
func NewRootObjectTree() *ObjectTree {
	return &ObjectTree{
		FullPath: "/",
		DirName:  "/",
	}
}

// CreateObjectTree creates an object tree.
func CreateObjectTree(objects []*Object) *ObjectTree {
	t := NewRootObjectTree()

	AddObjectsToTree(t, objects)

	return t
}

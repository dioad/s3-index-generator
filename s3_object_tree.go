package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/service/s3"
)

type Object struct {
	Object *s3.Object
	TagSet []*s3.Tag
}

func FetchObjects(s3Client *s3.S3, bucketName string, prefix string) ([]*Object, error) {
	return FetchObjectsWithContext(context.Background(), s3Client, bucketName, prefix)
}

func FetchObjectsWithContext(ctx context.Context, s3Client *s3.S3, bucketName string, prefix string) ([]*Object, error) {
	objInput := s3.ListObjectsInput{
		Bucket: &bucketName,
		Prefix: &prefix,
	}
	objects, err := s3Client.ListObjectsWithContext(ctx, &objInput)
	if err != nil {
		return nil, err
	}

	items := make([]*Object, 0)
	for _, o := range objects.Contents {
		tagInput := s3.GetObjectTaggingInput{
			Bucket: &bucketName,
			Key:    o.Key,
		}
		tags, err := s3Client.GetObjectTaggingWithContext(ctx, &tagInput)
		if err != nil {
			return nil, err
		}

		items = append(items, &Object{
			Object: o,
			TagSet: tags.TagSet,
		})
	}

	return items, nil
}

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
	if t.Exclusions.Include(*obj.Object.Key) {
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

type ByObjectKey []*s3.Object

func (k ByObjectKey) Len() int           { return len(k) }
func (k ByObjectKey) Swap(i, j int)      { k[i], k[j] = k[j], k[i] }
func (k ByObjectKey) Less(i, j int) bool { return *k[i].Key < *k[j].Key }

type ByChildKey []string

func (k ByChildKey) Len() int           { return len(k) }
func (k ByChildKey) Swap(i, j int)      { k[i], k[j] = k[j], k[i] }
func (k ByChildKey) Less(i, j int) bool { return k[i] < k[j] }

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

func AddObjectToTree(t *ObjectTree, obj *Object) {
	parts := strings.Split(*obj.Object.Key, "/")
	if len(parts) == 1 {
		t.AddObject(obj)
	} else {
		if parts[0] == t.PrefixToStrip {
			parts = parts[1:]
		}
		AddPathToTree(t, parts, obj)
	}
}

func AddObjectsToTree(t *ObjectTree, objects []*Object) {
	for _, o := range objects {
		AddObjectToTree(t, o)
	}
}

func NewRootObjectTree() *ObjectTree {
	return &ObjectTree{
		FullPath: "/",
		DirName:  "/",
	}
}

func CreateObjectTree(objects []*Object) *ObjectTree {
	t := NewRootObjectTree()

	AddObjectsToTree(t, objects)

	return t
}

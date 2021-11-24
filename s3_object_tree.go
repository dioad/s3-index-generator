package main

import (
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/service/s3"
)

type ObjectTree struct {
	Title         string // extract to Page
	Nonce         string // extract to Page
	FullPath      string
	DirName       string
	Objects       []*s3.Object
	Children      map[string]*ObjectTree
	Exclusions    Exclusions
	PrefixToStrip string
}

type ExcludeFunc func(string) bool

type Exclusions []ExcludeFunc

func (e Exclusions) Include(key string) bool {
	// log.Printf("Include: %v", key)
	for _, excludeFunc := range e {
		if excludeFunc(key) {
			return false
		}
	}
	return true
}

func ExcludeKey(key string) ExcludeFunc {
	return func(path string) bool {
		//log.Printf("ExcludeKey: comparing '%v' with '%v'", key, path)
		//log.Printf("  returning %v", key == path)
		return key == path
	}
}

func ExcludePrefix(prefix string) ExcludeFunc {
	return func(path string) bool {
		//log.Printf("ExcludePrefix: comparing '%v' with '%v'", prefix, path)
		//log.Printf("  returning %v", strings.HasPrefix(path, prefix))
		return strings.HasPrefix(path, prefix)
	}
}

func ExcludeSuffix(suffix string) ExcludeFunc {
	return func(path string) bool {
		//log.Printf("ExcludeSuffix: comparing '%v' with '%v'", suffix, path)
		//log.Printf("  returning %v", strings.HasSuffix(path, suffix))
		return strings.HasSuffix(path, suffix)
	}
}
func (t *ObjectTree) AddExclusion(f ExcludeFunc) {
	if t.Exclusions == nil {
		t.Exclusions = make(Exclusions, 0)
	}
	t.Exclusions = append(t.Exclusions, f)
}

func (t *ObjectTree) AddObject(obj *s3.Object) {
	if t.Objects == nil {
		t.Objects = make([]*s3.Object, 0)
	}

	//log.Printf("AddObject: %v", *obj.Key)
	if t.Exclusions.Include(*obj.Key) {
		//log.Printf("AddObject:Include: %v", *obj.Key)
		t.Objects = append(t.Objects, obj)
	}
}

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

func AddPathToTree(t *ObjectTree, pathParts []string, obj *s3.Object) {
	if len(pathParts) == 1 {
		t.AddObject(obj)
	} else {
		newTree := t.AddChild(pathParts[0])
		if newTree != nil {
			AddPathToTree(newTree, pathParts[1:], obj)
		}
	}
}

func AddObjectToTree(t *ObjectTree, obj *s3.Object) {
	parts := strings.Split(*obj.Key, "/")
	if len(parts) == 1 {
		t.AddObject(obj)
	} else {
		if parts[0] == t.PrefixToStrip {
			parts = parts[1:]
		}
		AddPathToTree(t, parts, obj)
	}
}

func AddObjectsToTree(t *ObjectTree, objects []*s3.Object) {
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

func CreateObjectTree(objects []*s3.Object) *ObjectTree {
	t := NewRootObjectTree()

	AddObjectsToTree(t, objects)

	return t
}

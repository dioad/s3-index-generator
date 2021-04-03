package main

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/service/s3"
)

type ObjectTree struct {
	FullPath string
	DirName  string
	Objects  []*s3.Object
	Children map[string]*ObjectTree
}

func (t *ObjectTree) AddObject(obj *s3.Object) {
	if t.Objects == nil {
		t.Objects = make([]*s3.Object, 0)
	}
	if !strings.HasSuffix(*obj.Key, "index.html") {
		if !strings.HasSuffix(*obj.Key, "/") {
			t.Objects = append(t.Objects, obj)
		}
	}
}

func (t *ObjectTree) AddChild(name string) *ObjectTree {
	if t.Children == nil {
		t.Children = make(map[string]*ObjectTree, 0)
	}
	if _, exists := t.Children[name]; !exists {
		t.Children[name] = &ObjectTree{
			DirName:  name,
			FullPath: filepath.Join(t.FullPath, name),
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

func printTree(out io.Writer, t *ObjectTree, indent int) {
	var prefix string
	for i := 0; i < indent; i++ {
		prefix = fmt.Sprintf("%v ", prefix)
	}

	childKeys := make([]string, 0, len(t.Children))
	for k := range t.Children {
		childKeys = append(childKeys, k)
	}

	sort.Sort(ByChildKey(childKeys))
	for _, c := range childKeys {
		fmt.Fprintf(out, "%v%v/\n", prefix, c)
		printTree(out, t.Children[c], indent+1)
	}

	sort.Sort(ByObjectKey(t.Objects))
	for _, o := range t.Objects {
		parts := strings.Split(*o.Key, "/")
		fmt.Fprintf(out, "%v%v\n", prefix, parts[len(parts)-1])
	}
}

func PrintTree(out io.Writer, t *ObjectTree) {
	printTree(out, t, 0)
}

func AddPathToTree(t *ObjectTree, pathParts []string, obj *s3.Object) {
	if len(pathParts) == 1 {
		t.AddObject(obj)
	} else {
		newTree := t.AddChild(pathParts[0])
		AddPathToTree(newTree, pathParts[1:], obj)
	}
}

func AddObjectToTree(t *ObjectTree, obj *s3.Object) {
	parts := strings.Split(*obj.Key, "/")
	if len(parts) == 1 {
		t.AddObject(obj)
	} else {
		AddPathToTree(t, parts, obj)
	}
}

func CreateObjectTree(objects []*s3.Object) *ObjectTree {
	t := &ObjectTree{
		FullPath: "/",
		DirName:  "/",
	}

	for _, o := range objects {
		AddObjectToTree(t, o)
	}

	return t
}

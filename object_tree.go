package main

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"
)

// ObjectTree is a tree of objects.
type ObjectTree struct {
	FullPath string
	DirName  string
	Objects  []Object
	Children map[string]*ObjectTree
	Config   ObjectTreeConfig
}

// AddChild adds a child to the tree, if it doesn't already exist.
func (t *ObjectTree) AddChild(name string) *ObjectTree {
	if t.Children == nil {
		t.Children = make(map[string]*ObjectTree)
	}

	if !t.Config.Exclusions.Include(name) {
		return nil
	}

	if _, exists := t.Children[name]; !exists {
		fullPath := filepath.Join(t.FullPath, name)
		t.Children[name] = NewObjectTree(t.Config, fullPath)
	}

	return t.Children[name]
}

// SortedChildren returns the sub-directories in display order: folders that are not versions first, by
// name, then version folders by semantic version, newest first. Ranging over Children directly yields
// plain string order, which puts 1.10.0 before 1.9.0.
func (t *ObjectTree) SortedChildren() []*ObjectTree {
	children := slices.Collect(maps.Values(t.Children))

	slices.SortFunc(children, func(a, b *ObjectTree) int {
		aVersion, aErr := ParseSemVer(a.DirName)
		bVersion, bErr := ParseSemVer(b.DirName)

		switch {
		case aErr != nil && bErr != nil:
			return strings.Compare(a.DirName, b.DirName)
		case aErr != nil:
			return -1
		case bErr != nil:
			return 1
		}

		// Newest first. Versions that compare equal (e.g. "1.0" and "1.0.0") fall back to name order.
		if c := bVersion.Compare(*aVersion); c != 0 {
			return c
		}

		return strings.Compare(a.DirName, b.DirName)
	})

	return children
}

func (t *ObjectTree) ParentName() string {
	return filepath.Base(t.ParentFullPath())
}

func (t *ObjectTree) ParentFullPath() string {
	return filepath.Clean(filepath.Join(t.FullPath, ".."))
}

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

	if t.Config.Exclusions.Include(obj.Key()) {
		t.Objects = append(t.Objects, obj)
	}
}

func (t *ObjectTree) AddObject(obj Object) {
	parts := strings.Split(obj.Key(), "/")
	if len(parts) == 1 {
		t.addSinglePartObject(obj)
	} else {
		if parts[0] == t.Config.PrefixToStrip {
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

func (t *ObjectTree) AddAllObjectsFromLister(ctx context.Context, objectLister ObjectListerFunc) error {
	objects, err := objectLister(ctx, t.Config.PrefixToStrip)
	if err != nil {
		return fmt.Errorf("error listing objects: %w", err)
	}

	t.AddObjects(objects)

	return nil
}

func (t *ObjectTree) AddObjectsWithPrefixFromLister(ctx context.Context, objectLister ObjectListerFunc, prefix string) error {
	objects, err := objectLister(ctx, filepath.Join(t.Config.PrefixToStrip, prefix))
	if err != nil {
		return fmt.Errorf("error listing objects: %w", err)
	}

	t.AddObjects(objects)

	return nil
}

type ObjectTreeWalker func(objTree *ObjectTree) error
type ObjectWalker func(obj *Object) error

func (t *ObjectTree) Walk(f ObjectTreeWalker, recursive bool, depthFirst bool) error {
	if !depthFirst {
		err := f(t)
		if err != nil {
			return err
		}
	}

	if recursive {
		errGroup := errgroup.Group{}
		errGroup.SetLimit(10)

		for _, v := range t.Children {
			errGroup.Go(func() error {
				return v.Walk(f, recursive, depthFirst)
			})
		}

		err := errGroup.Wait()
		if err != nil {
			return fmt.Errorf("failed to walk object tree: %w", err)
		}
	}

	if depthFirst {
		err := f(t)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *ObjectTree) WalkObjects(f ObjectWalker, recursive bool, depthFirst bool) error {
	if !depthFirst {
		err := t.walkLocalObjects(f)
		if err != nil {
			return err
		}
	}

	if recursive {
		errGroup := errgroup.Group{}
		errGroup.SetLimit(10)

		for _, v := range t.Children {
			errGroup.Go(func() error {
				return v.WalkObjects(f, recursive, depthFirst)
			})
		}

		err := errGroup.Wait()
		if err != nil {
			return fmt.Errorf("failed to walk objects: %w", err)
		}
	}

	if depthFirst {
		err := t.walkLocalObjects(f)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *ObjectTree) walkLocalObjects(f ObjectWalker) error {
	for _, v := range t.Objects {
		err := f(&v)
		if err != nil {
			return err
		}
	}

	return nil
}

func NewObjectTree(cfg ObjectTreeConfig, fullPath string) *ObjectTree {
	dirName := filepath.Base(fullPath)

	return &ObjectTree{
		Config:   cfg,
		FullPath: fullPath,
		DirName:  dirName,
	}
}

// NewRootObjectTree creates a new root object tree.
func NewRootObjectTree(cfg ObjectTreeConfig) *ObjectTree {
	return NewObjectTree(cfg, "/")
}

func NewObjectTreeWithObjects(cfg ObjectTreeConfig, objects []Object) *ObjectTree {
	t := NewRootObjectTree(cfg)

	t.AddObjects(objects)

	return t
}

func NewObjectTreeFromLister(ctx context.Context, cfg ObjectTreeConfig, objectLister ObjectListerFunc) (*ObjectTree, error) {
	t := NewRootObjectTree(cfg)

	err := t.AddAllObjectsFromLister(ctx, objectLister)
	if err != nil {
		return nil, err
	}

	return t, nil
}

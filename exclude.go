package main

import "strings"

// PredicateFunc is a function that excludes paths.
type PredicateFunc func(string) bool

// PathFilter is an interface for filtering paths.
type PathFilter interface {
	Include(string) bool
}

// Exclusions is a list of functions that exclude paths.
type Exclusions []PredicateFunc

// Include returns true if the path should be included.
func (e Exclusions) Include(key string) bool {
	for _, excludeFunc := range e {
		if excludeFunc(key) {
			return false
		}
	}
	return true
}

// Inclusions is a list of functions that exclude paths.
type Inclusions []PredicateFunc

// Include returns true if the path should be included.
func (i Inclusions) Include(key string) bool {
	for _, includeFunc := range i {
		if includeFunc(key) {
			return true
		}
	}
	return false
}

// HasKey returns a function that excludes paths with the given key.
func HasKey(key string) PredicateFunc {
	return func(path string) bool {
		//log.Printf("HasKey: comparing '%v' with '%v'", key, path)
		//log.Printf("  returning %v", key == path)
		return key == path
	}
}

// HasPrefix returns a function that excludes paths with the given prefix.
func HasPrefix(prefix string) PredicateFunc {
	return func(path string) bool {
		//log.Printf("HasPrefix: comparing '%v' with '%v'", prefix, path)
		//log.Printf("  returning %v", strings.HasPrefix(path, prefix))
		return strings.HasPrefix(path, prefix)
	}
}

// HasSuffix returns a function that excludes paths with the given suffix.
func HasSuffix(suffix string) PredicateFunc {
	return func(path string) bool {
		//log.Printf("HasSuffix: comparing '%v' with '%v'", suffix, path)
		//log.Printf("  returning %v", strings.HasSuffix(path, suffix))
		return strings.HasSuffix(path, suffix)
	}
}

package main

import (
	"testing"
)

func TestIndexFormats(t *testing.T) {
	// Test the index formats
	format := "json"

	formats := indexFormats(format)
	if len(formats) != 1 {
		t.Errorf("Expected 1, got %d", len(formats))
	}
}

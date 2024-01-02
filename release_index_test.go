package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		KeyExtractions: ReleaseDetailKeyExtractions{
			DefaultReleaseInfoKeyExtractor,
		},
	}
	obj := simpleObject("TestProduct/1.0.0/TestProduct_linux_amd64.zip")

	entry, err := NewIndexEntry(cfg, obj)
	if err != nil {
		t.Fatalf("NewIndexEntry() failed, %v", err)
	}

	if entry.Arch != "amd64" || entry.Name != "TestProduct" || entry.Os != "linux" || entry.Version != "1.0.0" {
		t.Errorf("NewIndexEntry() failed, entry fields not correctly set")
	}
}

func TestNewVersionIndex(t *testing.T) {
	cfg := IndexConfig{
		KeyExtractions: ReleaseDetailKeyExtractions{
			DefaultReleaseInfoKeyExtractor,
		},
	}
	obj := simpleObject("TestProduct/1.0.0/TestProduct_linux_amd64.zip")

	rootTree := NewRootObjectTree(ObjectTreeConfig{PrefixToStrip: "data"})
	rootTree.AddObject(obj)

	versionIndex := NewVersionIndex(cfg, rootTree.Children["TestProduct"].Children["1.0.0"])

	if versionIndex.Name == "" || versionIndex.Version != "1.0.0" || len(versionIndex.Builds) != 1 {
		t.Errorf("NewVersionIndex() failed, versionIndex fields not correctly set")
	}
}

func TestExtractMetadataFromKey(t *testing.T) {
	tests := []struct {
		key      string
		re       string
		expected map[string]string
	}{
		{
			key: "data/connect/0.57.1/connect_linux_arm64.tar.gz",
			re:  `(?P<Prefix>.*?/)?(?P<Product>[^/]+)/(?P<Version>[^/]+)/(?P<PackageName>[^_]+)_(?P<Extra>[^_]*?_)?(?P<OS>[^_\d]+)_(?P<Arch>[^\.]+)\.(?P<ArchiveType>[^/]+)$`,
			expected: map[string]string{
				"Prefix":      "data/",
				"Product":     "connect",
				"Version":     "0.57.1",
				"PackageName": "connect",
				"Extra":       "",
				"OS":          "linux",
				"Arch":        "arm64",
				"ArchiveType": "tar.gz",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			extractMetadataFromKeyHelper(t, tc.re, tc.key, tc.expected)
		})
	}
}

func extractMetadataFromKeyHelper(t *testing.T, re string, key string, expected map[string]string) {
	t.Helper()

	results, err := extractMetadataFromKey(re, key)
	if err != nil {
		t.Fatalf("extractMetadataFromKey() failed, %v", err)
	}

	assert.Equal(t, expected, results)

	for k, v := range expected {
		if results[k] != v {
			t.Errorf("field %v: expected %v, got %v", k, expected[k], results[k])
		}
	}
}

func TestExtractReleaseInfoFromKey(t *testing.T) {
	// key := "data/connect/0.57.1/connect_linux_arm64.tar.gz"
	// key := data/connect/0.57.1/connect_windows_amd64.zip

	tests := []struct {
		key      string
		expected map[string]string
	}{
		{
			key: "data/connect/0.57.1/connect_linux_arm64.tar.gz",
			expected: map[string]string{
				"ArchiveType": "tar.gz",
				"Arch":        "arm64",
				"Product":     "connect",
				"OS":          "linux",
				"PackageName": "connect",
			},
		},
		{
			key: "data/connect/0.57.1/connect_windows_amd64.zip",
			expected: map[string]string{
				"ArchiveType": "zip",
				"Arch":        "amd64",
				"Product":     "connect",
				"OS":          "windows",
				"PackageName": "connect",
			},
		},
		{
			key: "data/connect/0.57.1/connect_darwin_all.dmg",
			expected: map[string]string{
				"ArchiveType": "dmg",
				"Arch":        "all",
				"Product":     "connect",
				"OS":          "darwin",
				"PackageName": "connect",
			},
		},
		{
			key: "data/connect/0.57.1/dioad-connect_0.57.1_linux_amd64.deb",
			expected: map[string]string{
				"ArchiveType": "deb",
				"Arch":        "amd64",
				"Product":     "connect",
				"OS":          "linux",
				"PackageName": "dioad-connect",
				"Extra":       "0.57.1",
			},
		},
		{
			key: "data/connect/0.57.1/dioad-connect_0.57.1_linux_amd64.rpm",
			expected: map[string]string{
				"ArchiveType": "rpm",
				"Arch":        "amd64",
				"Product":     "connect",
				"OS":          "linux",
				"PackageName": "dioad-connect",
				"Extra":       "0.57.1",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			extractReleaseInfoHelper(t, tc.key, tc.expected)
		})
	}
}

func extractReleaseInfoHelper(t *testing.T, key string, expected map[string]string) {
	t.Helper()

	extractions := ReleaseDetailKeyExtractions{
		DefaultReleaseInfoKeyExtractor,
	}

	results, err := extractions.ExtractReleaseDetails(key)
	if err != nil {
		t.Fatalf("ExtractReleaseDetails() failed, %v", err)
	}

	// assert that keys in expected are in results
	for k, v := range expected {
		if results[k] != v {
			t.Errorf("field %v: expected %v, got %v", k, expected[k], results[k])
		}
	}

}

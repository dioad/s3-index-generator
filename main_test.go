package main

import (
	"os"
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

func TestIndexFormatsMultiple(t *testing.T) {
	formats := indexFormats("html,json")
	if len(formats) != 2 {
		t.Errorf("expected 2 formats, got %d", len(formats))
	}
}

func TestIndexFormatsUnknown(t *testing.T) {
	formats := indexFormats("xml")
	if len(formats) != 0 {
		t.Errorf("expected 0 formats for unknown type, got %d", len(formats))
	}
}

func TestParseConfigFromEnvironment_Defaults(t *testing.T) {
	// Unset vars so we test defaults, not inherit from the environment.
	for _, key := range []string{"BUCKET", "INDEX_TYPE", "INDEX_FORMATS", "INDEX_TEMPLATE", "TEMPLATE_BUCKET_URL", "STATIC_BUCKET_URL", "SSE", "OBJECT_PREFIX", "DESTINATION_BUCKET_PREFIX"} {
		if orig, exists := os.LookupEnv(key); exists {
			_ = os.Unsetenv(key)
			k := key
			t.Cleanup(func() { _ = os.Setenv(k, orig) })
		}
	}

	cfg, err := parseConfigFromEnvironment()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.IndexType != MultiPageIdentifier {
		t.Errorf("expected default IndexType %q, got %q", MultiPageIdentifier, cfg.IndexType)
	}
	if len(cfg.IndexFormats) != 2 {
		t.Errorf("expected 2 default index formats, got %d", len(cfg.IndexFormats))
	}
}

func TestParseConfigFromEnvironment_InvalidIndexType(t *testing.T) {
	t.Setenv("INDEX_TYPE", "badvalue")
	defer t.Setenv("INDEX_TYPE", "")

	_, err := parseConfigFromEnvironment()
	if err == nil {
		t.Error("expected error for invalid INDEX_TYPE, got nil")
	}
}

func TestParseConfigFromEnvironment_InvalidTemplateBucketURL(t *testing.T) {
	t.Setenv("TEMPLATE_BUCKET_URL", "http://not-s3/path")
	defer t.Setenv("TEMPLATE_BUCKET_URL", "")

	_, err := parseConfigFromEnvironment()
	if err == nil {
		t.Error("expected error for non-s3 TEMPLATE_BUCKET_URL, got nil")
	}
}

func TestParseConfigFromEnvironment_InvalidStaticBucketURL(t *testing.T) {
	t.Setenv("STATIC_BUCKET_URL", "http://not-s3/path")
	defer t.Setenv("STATIC_BUCKET_URL", "")

	_, err := parseConfigFromEnvironment()
	if err == nil {
		t.Error("expected error for non-s3 STATIC_BUCKET_URL, got nil")
	}
}

func TestParseConfigFromEnvironment_ValidS3URLs(t *testing.T) {
	t.Setenv("TEMPLATE_BUCKET_URL", "s3://my-bucket/templates")
	t.Setenv("STATIC_BUCKET_URL", "s3://my-bucket/static")
	defer func() {
		t.Setenv("TEMPLATE_BUCKET_URL", "")
		t.Setenv("STATIC_BUCKET_URL", "")
	}()

	cfg, err := parseConfigFromEnvironment()
	if err != nil {
		t.Fatalf("expected no error for valid s3 URLs, got %v", err)
	}
	if cfg.TemplateBucketURL == nil {
		t.Error("expected TemplateBucketURL to be set")
	}
	if cfg.StaticBucketURL == nil {
		t.Error("expected StaticBucketURL to be set")
	}
}

func TestParseConfigFromEnvironment_IndexTemplate(t *testing.T) {
	t.Setenv("INDEX_TYPE", "singlepage")
	defer t.Setenv("INDEX_TYPE", "")

	cfg, err := parseConfigFromEnvironment()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.IndexTemplate != "singlepage.index.html.tmpl" {
		t.Errorf("expected singlepage template, got %q", cfg.IndexTemplate)
	}
}

func TestParseConfigFromEnvironment_ReleaseKeyPatterns(t *testing.T) {
	t.Setenv("RELEASE_KEY_PATTERNS", `(?P<Product>[^/]+)/(?P<Version>[^/]+)/a.zip,(?P<Product>[^/]+)/b.zip`)
	defer t.Setenv("RELEASE_KEY_PATTERNS", "")

	cfg, err := parseConfigFromEnvironment()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(cfg.ReleaseKeyPatterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(cfg.ReleaseKeyPatterns))
	}
}

func TestBuildIndexConfig_Default(t *testing.T) {
	cfg, err := buildIndexConfig(nil)
	if err != nil {
		t.Fatalf("buildIndexConfig(nil) failed: %v", err)
	}
	if len(cfg.KeyExtractions) != 1 {
		t.Errorf("expected 1 default extraction, got %d", len(cfg.KeyExtractions))
	}
}

func TestBuildIndexConfig_CustomPatterns(t *testing.T) {
	patterns := []string{
		`(?P<Product>[^/]+)/(?P<Version>[^/]+)/(?P<OS>[^_]+)_(?P<Arch>[^.]+)\.zip`,
	}
	cfg, err := buildIndexConfig(patterns)
	if err != nil {
		t.Fatalf("buildIndexConfig() failed: %v", err)
	}
	if len(cfg.KeyExtractions) != 1 {
		t.Errorf("expected 1 custom extraction, got %d", len(cfg.KeyExtractions))
	}

	// Verify the custom pattern is actually used.
	result, err := cfg.KeyExtractions.ExtractReleaseDetails("myproduct/2.0.0/linux_amd64.zip")
	if err != nil {
		t.Fatalf("custom pattern failed to match: %v", err)
	}
	if result["Product"] != "myproduct" || result["Version"] != "2.0.0" {
		t.Errorf("unexpected extraction result: %v", result)
	}
}

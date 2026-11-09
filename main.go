package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"runtime"
	"runtime/pprof"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/afero"
)

type IndexFormat string

var (
	SinglePageIdentifier = "singlepage"
	MultiPageIdentifier  = "multipage"

	JSONIndex IndexFormat = "json"
	HTMLIndex IndexFormat = "html"
)

type Config struct {
	// Bucket is the S3 bucket to be indexed
	Bucket string
	// BucketDestinationPrefix is the prefix within the bucket to root the generated indexes
	DestinationBucketPrefix string
	// ObjectPrefix is the prefix of the S3 objects to be indexed
	ObjectPrefix         string
	TemplateBucketURL    *url.URL
	StaticBucketURL      *url.URL
	IndexType            string
	IndexTemplate        string
	IndexFormats         []IndexFormat
	ServerSideEncryption string
	LocalOutputDirectory string
	// ReleaseKeyPatterns is an ordered list of regex patterns used to extract release metadata
	// from S3 object keys. Each pattern is tried in order and the first match wins. When empty,
	// the built-in default pattern is used. See DefaultReleaseInfoKeyExtractor for the format.
	ReleaseKeyPatterns []string
}

// s3URLFromEnvironment parses the named environment variable as an s3:// URL.
// It returns nil when the variable is not set.
func s3URLFromEnvironment(name string) (*url.URL, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return nil, nil //nolint:nilnil // an unset variable is not an error and has no URL
	}

	u, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", name, err)
	}
	if u.Scheme != "s3" {
		return nil, fmt.Errorf("invalid %s: expected s3:// scheme, got %q", name, u.Scheme)
	}

	return u, nil
}

// indexFormatsFromEnvironment returns the formats listed in INDEX_FORMATS, defaulting to HTML and JSON
// when it is unset or lists no known format.
func indexFormatsFromEnvironment() []IndexFormat {
	if value, ok := os.LookupEnv("INDEX_FORMATS"); ok {
		if formats := indexFormats(value); len(formats) > 0 {
			return formats
		}
	}

	return []IndexFormat{HTMLIndex, JSONIndex}
}

func parseConfigFromEnvironment() (Config, error) {
	var cfg Config

	var ok bool

	cfg.Bucket, _ = os.LookupEnv("BUCKET")
	cfg.DestinationBucketPrefix, _ = os.LookupEnv("DESTINATION_BUCKET_PREFIX")
	cfg.ObjectPrefix, _ = os.LookupEnv("OBJECT_PREFIX")

	if cfg.IndexType, ok = os.LookupEnv("INDEX_TYPE"); !ok {
		cfg.IndexType = MultiPageIdentifier
	} else if cfg.IndexType != MultiPageIdentifier && cfg.IndexType != SinglePageIdentifier {
		return cfg, fmt.Errorf("invalid INDEX_TYPE %q: expected %q or %q", cfg.IndexType, MultiPageIdentifier, SinglePageIdentifier)
	}

	cfg.IndexFormats = indexFormatsFromEnvironment()

	if cfg.IndexTemplate, ok = os.LookupEnv("INDEX_TEMPLATE"); !ok {
		cfg.IndexTemplate = fmt.Sprintf("%v.index.html.tmpl", cfg.IndexType)
	}

	var err error

	if cfg.TemplateBucketURL, err = s3URLFromEnvironment("TEMPLATE_BUCKET_URL"); err != nil {
		return cfg, err
	}

	if cfg.StaticBucketURL, err = s3URLFromEnvironment("STATIC_BUCKET_URL"); err != nil {
		return cfg, err
	}

	// Can we figure these details out by looking at bucket config?
	cfg.ServerSideEncryption, _ = os.LookupEnv("SSE")

	if releaseKeyPatterns, ok := os.LookupEnv("RELEASE_KEY_PATTERNS"); ok && releaseKeyPatterns != "" {
		cfg.ReleaseKeyPatterns = strings.Split(releaseKeyPatterns, ",")
	}

	return cfg, cfg.validate()
}

func indexFormats(indexFormat string) []IndexFormat {
	formats := make([]IndexFormat, 0)
	indexFormatStrings := strings.SplitSeq(indexFormat, ",")
	for format := range indexFormatStrings {
		if format == "json" {
			formats = append(formats, JSONIndex)
		}
		if format == "html" {
			formats = append(formats, HTMLIndex)
		}
	}

	return formats
}

// generatedIndexFiles are the file names the generator writes into every directory.
var generatedIndexFiles = []string{"index.html", "index.json"}

// skipReason explains why an event for key must not trigger a run, or returns "" if it should. Keys the
// generator writes itself (index files and static assets) are skipped so its own output can never start
// another run, whatever the destination prefix is.
func skipReason(cfg Config, key string) string {
	if !strings.HasPrefix(key, cfg.ObjectPrefix) {
		return fmt.Sprintf("does not match prefix %q", cfg.ObjectPrefix)
	}

	if slices.Contains(generatedIndexFiles, path.Base(key)) {
		return "is a generated index file"
	}

	if strings.HasPrefix(key, path.Join(cfg.DestinationBucketPrefix, "static")+"/") {
		return "is a generated static asset"
	}

	return ""
}

// validate rejects configurations in which the generator's output would land inside the prefix whose
// events trigger it, which would make every run start another.
func (c Config) validate() error {
	if c.ObjectPrefix == "" {
		return nil
	}

	objectPrefix := strings.Trim(c.ObjectPrefix, "/") + "/"
	destinationPrefix := strings.Trim(c.DestinationBucketPrefix, "/") + "/"
	if strings.HasPrefix(destinationPrefix, objectPrefix) {
		return fmt.Errorf("DESTINATION_BUCKET_PREFIX %q is inside OBJECT_PREFIX %q: generated indexes would trigger further runs",
			c.DestinationBucketPrefix, c.ObjectPrefix)
	}

	return nil
}

// recordsToIndex returns how many records in the event warrant an index run, logging each one it skips.
func recordsToIndex(cfg Config, event events.S3Event) int {
	count := 0

	for _, record := range event.Records {
		key := record.S3.Object.Key
		if reason := skipReason(cfg, key); reason != "" {
			fmt.Printf("skipping: key %v %v\n", key, reason)

			continue
		}

		fmt.Printf("record: bucket: %v, key: %v, event: %v\n", record.S3.Bucket.Name, key, record.EventName)
		count++
	}

	return count
}

// HandleRequest returns the Lambda handler for S3 events. The index is rebuilt in full, so one run
// covers every record in the event however many qualify.
func HandleRequest(client *s3.Client, cfg Config) func(ctx context.Context, event events.S3Event) error {
	return func(ctx context.Context, event events.S3Event) error {
		fmt.Printf("records length: %d\n", len(event.Records))

		if recordsToIndex(cfg, event) == 0 {
			return nil
		}

		outputFS := NewS3OutputFS(client, cfg.Bucket, cfg.DestinationBucketPrefix, &cfg.ServerSideEncryption)

		return indexS3Bucket(ctx, client, cfg, outputFS)
	}
}

// Pipeline holds the dependencies for a single index generation run.
type Pipeline struct {
	client    *s3.Client
	cfg       Config
	outputFS  afero.Fs
	renderers IndexRenderers
}

// newPipeline constructs a Pipeline, resolving templates and static assets.
func newPipeline(_ context.Context, client *s3.Client, cfg Config, outputFS afero.Fs) (*Pipeline, error) {
	renderers, err := indexRenderers(client, cfg)
	if err != nil {
		return nil, err
	}

	if slices.Contains(cfg.IndexFormats, HTMLIndex) {
		if err := CopyStaticFiles(client, outputFS, cfg.StaticBucketURL); err != nil {
			return nil, fmt.Errorf("failed to copy static files: %w", err)
		}
	}

	return &Pipeline{
		client:    client,
		cfg:       cfg,
		outputFS:  outputFS,
		renderers: renderers,
	}, nil
}

// buildObjectTree lists all bucket objects and constructs the tree representation.
func (p *Pipeline) buildObjectTree(ctx context.Context) (*ObjectTree, error) {
	s3Bucket := NewS3Bucket(p.client, p.cfg.Bucket, p.cfg.ServerSideEncryption)

	objectTreeCfg := ObjectTreeConfig{
		PrefixToStrip: p.cfg.ObjectPrefix,
		Exclusions: Exclusions{
			HasKey("favicon.ico"),
			HasKey("index.html"),
			HasPrefix("."),
			HasSuffix("/"),
			HasSuffix("/index.html"),
		},
	}
	objectTree := NewRootObjectTree(objectTreeCfg)

	duration, err := TimeFunc(func() error {
		return objectTree.AddAllObjectsFromLister(ctx, s3Bucket.ListObjects)
	})
	log.Printf("CreateObjectTree: duration:%v\n", duration)
	if err != nil {
		return nil, fmt.Errorf("failed to create object tree: %w", err)
	}

	return objectTree, nil
}

// renderIndexes walks the object tree and writes index files to the output filesystem.
func (p *Pipeline) renderIndexes(objectTree *ObjectTree) error {
	recursive := p.cfg.IndexType != SinglePageIdentifier

	duration, err := TimeFunc(func() error {
		return RenderObjectTreeIndexes(objectTree, p.renderers, p.outputFS, recursive)
	})
	log.Printf("RenderObjectTreeIndexes: duration:%v\n", duration)
	if err != nil {
		return fmt.Errorf("failed to render object tree indexes: %w", err)
	}

	return nil
}

func indexS3Bucket(ctx context.Context, client *s3.Client, cfg Config, outputFS afero.Fs) error {
	pipeline, err := newPipeline(ctx, client, cfg, outputFS)
	if err != nil {
		return err
	}

	objectTree, err := pipeline.buildObjectTree(ctx)
	if err != nil {
		return err
	}

	return pipeline.renderIndexes(objectTree)
}

func indexRenderers(client *s3.Client, cfg Config) (IndexRenderers, error) {
	renderers := make(IndexRenderers, 0)

	indexCfg, err := buildIndexConfig(cfg.ReleaseKeyPatterns)
	if err != nil {
		return nil, fmt.Errorf("invalid release key patterns: %w", err)
	}

	for _, format := range cfg.IndexFormats {
		switch format {
		case JSONIndex:
			renderers = append(renderers, JSONIndexRenderer(indexCfg))
		case HTMLIndex:
			tmpl, err := LoadTemplates(client, cfg.TemplateBucketURL)
			if err != nil {
				return nil, fmt.Errorf("failed to load templates: %w", err)
			}
			renderers = append(renderers, HTMLIndexRenderer(tmpl, cfg.IndexTemplate))
		}
	}

	return renderers, nil
}

func TimeFunc(f func() error) (time.Duration, error) {
	start := time.Now()
	err := f()

	return time.Since(start), err
}

func localOutputFS(args []string) (afero.Fs, error) {
	var outputFS afero.Fs
	var err error
	if len(os.Args) == 3 {
		localOutputDirectory := args[2]
		outputFS, err = NewLocalOutputFS(localOutputDirectory)
		if err != nil {
			return nil, fmt.Errorf("failed to create local output FS: %w", err)
		}
	}

	return outputFS, nil
}

func main() {
	ctx := context.Background()

	client, err := newS3Client(ctx)
	if err != nil {
		log.Fatalf("failed to create S3 client: %v", err)
	}

	cfg, err := parseConfigFromEnvironment()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	if os.Getenv("_HANDLER") != "" {
		if cfg.Bucket == "" {
			log.Fatalf("BUCKET environment variable is required in Lambda mode")
		}
		lambda.Start(HandleRequest(client, cfg))

		return
	}

	if len(os.Args) >= 2 {
		if err := runCLI(ctx, client, cfg, os.Args); err != nil {
			log.Fatal(err)
		}
	}
}

// runCLI indexes the bucket named by args[1] ("bucket[/prefix]"), writing to args[2] when given or
// back to the bucket otherwise, and records CPU and heap profiles of the run.
func runCLI(ctx context.Context, client *s3.Client, cfg Config, args []string) error {
	cfg.Bucket, cfg.DestinationBucketPrefix, _ = strings.Cut(args[1], "/")

	if err := cfg.validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	outputFS, err := localOutputFS(args)
	if err != nil {
		return fmt.Errorf("failed to create local output FS: %w", err)
	}

	if outputFS == nil {
		outputFS = NewS3OutputFS(client, cfg.Bucket, "", &cfg.ServerSideEncryption)
	}

	cpuProf, err := os.Create("cpu.pprof")
	if err != nil {
		return err
	}
	defer func() { _ = cpuProf.Close() }()

	if err := pprof.StartCPUProfile(cpuProf); err != nil {
		return err
	}
	defer pprof.StopCPUProfile()

	heapProf, err := os.Create("heap.pprof")
	if err != nil {
		return err
	}
	defer func() { _ = heapProf.Close() }()

	if err := indexS3Bucket(ctx, client, cfg, outputFS); err != nil {
		return fmt.Errorf("failed to generate index files: %w", err)
	}

	runtime.GC()
	if err := pprof.WriteHeapProfile(heapProf); err != nil {
		log.Printf("failed to write heap profile: %v", err)
	}

	return nil
}

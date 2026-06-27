package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws/session"
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

func parseConfigFromEnvironment() (Config, error) {
	var cfg Config

	var ok bool

	cfg.Bucket, _ = os.LookupEnv("BUCKET")
	cfg.DestinationBucketPrefix, _ = os.LookupEnv("DESTINATION_BUCKET_PREFIX")
	cfg.ObjectPrefix, _ = os.LookupEnv("OBJECT_PREFIX")

	if cfg.IndexType, ok = os.LookupEnv("INDEX_TYPE"); !ok {
		cfg.IndexType = MultiPageIdentifier
	} else {
		if cfg.IndexType != MultiPageIdentifier && cfg.IndexType != SinglePageIdentifier {
			return cfg, fmt.Errorf("invalid INDEX_TYPE %q: expected %q or %q", cfg.IndexType, MultiPageIdentifier, SinglePageIdentifier)
		}
	}

	if indexFormatValue, ok := os.LookupEnv("INDEX_FORMATS"); ok {
		cfg.IndexFormats = indexFormats(indexFormatValue)
	}

	if len(cfg.IndexFormats) == 0 {
		cfg.IndexFormats = []IndexFormat{HTMLIndex, JSONIndex}
	}

	if cfg.IndexTemplate, ok = os.LookupEnv("INDEX_TEMPLATE"); !ok {
		cfg.IndexTemplate = fmt.Sprintf("%v.index.html.tmpl", cfg.IndexType)
	}

	if templateBucketURLString, ok := os.LookupEnv("TEMPLATE_BUCKET_URL"); ok {
		tmpURL, err := url.Parse(templateBucketURLString)
		if err != nil {
			return cfg, fmt.Errorf("invalid TEMPLATE_BUCKET_URL: %w", err)
		}
		if tmpURL.Scheme != "s3" {
			return cfg, fmt.Errorf("invalid TEMPLATE_BUCKET_URL: expected s3:// scheme, got %q", tmpURL.Scheme)
		}
		cfg.TemplateBucketURL = tmpURL
	}

	if staticBucketURLString, ok := os.LookupEnv("STATIC_BUCKET_URL"); ok {
		tmpURL, err := url.Parse(staticBucketURLString)
		if err != nil {
			return cfg, fmt.Errorf("invalid STATIC_BUCKET_URL: %w", err)
		}
		if tmpURL.Scheme != "s3" {
			return cfg, fmt.Errorf("invalid STATIC_BUCKET_URL: expected s3:// scheme, got %q", tmpURL.Scheme)
		}
		cfg.StaticBucketURL = tmpURL
	}

	// Can we figure these details out by looking at bucket config?
	cfg.ServerSideEncryption, _ = os.LookupEnv("SSE")

	if releaseKeyPatterns, ok := os.LookupEnv("RELEASE_KEY_PATTERNS"); ok && releaseKeyPatterns != "" {
		cfg.ReleaseKeyPatterns = strings.Split(releaseKeyPatterns, ",")
	}

	return cfg, nil
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

func HandleRequest(sess *session.Session, cfg Config) func(ctx context.Context, event events.S3Event) error {
	return func(ctx context.Context, event events.S3Event) error {
		if len(event.Records) == 0 {
			return nil
		}

		s3Entity := event.Records[0].S3
		// lc, _ := lambdacontext.FromContext(ctx)
		fmt.Printf("records length: %d", len(event.Records))
		fmt.Printf(" record[0]: bucket: %v, key: %v, event: %v",
			s3Entity.Bucket.Name,
			s3Entity.Object.Key,
			event.Records[0].EventName,
		)

		if !strings.HasPrefix(s3Entity.Object.Key, cfg.ObjectPrefix) {
			fmt.Printf("skipping: key %v does not match prefix %v", s3Entity.Object.Key, cfg.ObjectPrefix)
			return nil
		}

		outputFS := NewS3OutputFS(sess, cfg.Bucket, cfg.DestinationBucketPrefix, &cfg.ServerSideEncryption)

		return indexS3Bucket(ctx, sess, cfg, outputFS)
	}
}

// Pipeline holds the dependencies for a single index generation run.
type Pipeline struct {
	sess      *session.Session
	cfg       Config
	outputFS  afero.Fs
	renderers IndexRenderers
}

// newPipeline constructs a Pipeline, resolving templates and static assets.
func newPipeline(ctx context.Context, sess *session.Session, cfg Config, outputFS afero.Fs) (*Pipeline, error) {
	renderers, err := indexRenderers(sess, cfg)
	if err != nil {
		return nil, err
	}

	if slices.Contains(cfg.IndexFormats, HTMLIndex) {
		if err := CopyStaticFiles(sess, outputFS, cfg.StaticBucketURL); err != nil {
			return nil, fmt.Errorf("failed to copy static files: %w", err)
		}
	}

	return &Pipeline{
		sess:      sess,
		cfg:       cfg,
		outputFS:  outputFS,
		renderers: renderers,
	}, nil
}

// buildObjectTree lists all bucket objects and constructs the tree representation.
func (p *Pipeline) buildObjectTree(ctx context.Context) (*ObjectTree, error) {
	s3Bucket := NewS3Bucket(p.sess, p.cfg.Bucket, p.cfg.ServerSideEncryption)

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

func indexS3Bucket(ctx context.Context, sess *session.Session, cfg Config, outputFS afero.Fs) error {
	pipeline, err := newPipeline(ctx, sess, cfg, outputFS)
	if err != nil {
		return err
	}

	objectTree, err := pipeline.buildObjectTree(ctx)
	if err != nil {
		return err
	}

	return pipeline.renderIndexes(objectTree)
}

func indexRenderers(sess *session.Session, cfg Config) (IndexRenderers, error) {
	renderers := make(IndexRenderers, 0)

	indexCfg := buildIndexConfig(cfg.ReleaseKeyPatterns)

	for _, format := range cfg.IndexFormats {
		switch format {
		case JSONIndex:
			renderers = append(renderers, JSONIndexRenderer(indexCfg))
		case HTMLIndex:
			tmpl, err := LoadTemplates(sess, cfg.TemplateBucketURL)
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
	//
	// cpuProf, err := os.Create("cpu.pprof")
	// heapProf, err := os.Create("heap.pprof")
	// if err != nil {
	//	log.Fatal(err)
	// }
	// pprof.StartCPUProfile(cpuProf)
	//
	// defer pprof.StopCPUProfile()
	//
	sess := s3Session()

	cfg, err := parseConfigFromEnvironment()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	if os.Getenv("_HANDLER") != "" {
		if cfg.Bucket == "" {
			log.Fatalf("BUCKET environment variable is required in Lambda mode")
		}
		lambda.Start(HandleRequest(sess, cfg))
	} else {
		if len(os.Args) >= 2 {
			cfg.Bucket, cfg.DestinationBucketPrefix, _ = strings.Cut(os.Args[1], "/")

			outputFS, err := localOutputFS(os.Args)
			if err != nil {
				log.Fatalf("failed to create local output FS: %v", err)
			}

			if outputFS == nil {
				outputFS = NewS3OutputFS(sess, cfg.Bucket, "", &cfg.ServerSideEncryption)
			}

			err = indexS3Bucket(context.Background(), sess, cfg, outputFS)
			//	pprof.WriteHeapProfile(heapProf)
			if err != nil {
				log.Fatalf("failed to generate index files: %v", err)
			}
		}
	}
}

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
}

func parseConfigFromEnvironment() Config {
	var cfg Config

	var ok bool

	cfg.Bucket, _ = os.LookupEnv("BUCKET")
	cfg.DestinationBucketPrefix, _ = os.LookupEnv("DESTINATION_BUCKET_PREFIX")
	cfg.ObjectPrefix, _ = os.LookupEnv("OBJECT_PREFIX")

	if cfg.IndexType, ok = os.LookupEnv("INDEX_TYPE"); !ok {
		cfg.IndexType = MultiPageIdentifier
	} else {
		if cfg.IndexType != MultiPageIdentifier && cfg.IndexType != SinglePageIdentifier {
			log.Fatalf("err: expected multipage or singlepage, found %v", cfg.IndexType)
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
			log.Fatalf("err: unable to parse TEMPLATE_BUCKET_URL as URL: %v", err)
		}
		cfg.TemplateBucketURL = tmpURL
	}

	if staticBucketURLString, ok := os.LookupEnv("STATIC_BUCKET_URL"); ok {
		tmpURL, err := url.Parse(staticBucketURLString)
		if err != nil {
			log.Fatalf("err: unable to parse STATIC_BUCKET_URL as URL: %v", err)
		}
		cfg.StaticBucketURL = tmpURL
	}

	// Can we figure these details out by looking at bucket config?
	cfg.ServerSideEncryption, _ = os.LookupEnv("SSE")

	return cfg
}

func indexFormats(indexFormat string) []IndexFormat {
	formats := make([]IndexFormat, 0)
	indexFormatStrings := strings.Split(indexFormat, ",")
	for _, format := range indexFormatStrings {
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
		//lc, _ := lambdacontext.FromContext(ctx)
		fmt.Printf("records length: %d", len(event.Records))
		fmt.Printf("record[0]: bucket: %v, key: %v, event: %v",
			event.Records[0].S3.Bucket.Name,
			event.Records[0].S3.Object.Key,
			event.Records[0].EventName,
		)

		if !strings.HasPrefix(event.Records[0].S3.Object.Key, cfg.ObjectPrefix) {
			fmt.Printf("skipping: key %v does not match prefix %v", event.Records[0].S3.Object.Key, cfg.ObjectPrefix)
			return nil
		}

		outputFS := NewS3OutputFS(sess, cfg.Bucket, cfg.DestinationBucketPrefix, &cfg.ServerSideEncryption)

		err := indexS3Bucket(ctx, sess, cfg, outputFS)
		if err != nil {
			return err
		}

		return err
	}
}

func indexS3Bucket(ctx context.Context, sess *session.Session, cfg Config, outputFS afero.Fs) error {
	s3Bucket := NewS3Bucket(sess, cfg.Bucket, cfg.ServerSideEncryption)

	renderers, err := indexRenderers(sess, cfg)
	if err != nil {
		return err
	}

	if slices.Contains(cfg.IndexFormats, HTMLIndex) {
		err := CopyStaticFiles(sess, outputFS, cfg.StaticBucketURL)
		if err != nil {
			return fmt.Errorf("failed to copy static files: %w", err)
		}
	}

	objectTreeCfg := ObjectTreeConfig{
		PrefixToStrip: cfg.ObjectPrefix,
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
		// return objectTree.AddObjectsWithPrefixFromLister(ctx, s3Bucket.ListObjectsWithTags, "s3-index-generator/")
		return objectTree.AddAllObjectsFromLister(ctx, s3Bucket.ListObjects)
	})
	log.Printf("CreateObjectTree: duration:%v\n", duration)
	if err != nil {
		return fmt.Errorf("failed to create object tree: %w", err)
	}

	// select renderer
	recursive := true
	if cfg.IndexType == SinglePageIdentifier {
		recursive = false
	}
	// end select renderer

	duration, err = TimeFunc(func() error {
		return RenderObjectTreeIndexes(objectTree, renderers, outputFS, recursive)
	})
	log.Printf("RenderObjectTreeIndexes: duration:%v\n", duration)
	if err != nil {
		return fmt.Errorf("failed to render object tree indexes: %w", err)
	}

	return nil
}

func indexRenderers(sess *session.Session, cfg Config) (IndexRenderers, error) {
	renderers := make(IndexRenderers, 0)

	for _, format := range cfg.IndexFormats {
		switch format {
		case JSONIndex:
			renderers = append(renderers, JSONIndexRenderer(DioadIndexConfig))
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
	//cpuProf, err := os.Create("cpu.pprof")
	//heapProf, err := os.Create("heap.pprof")
	//if err != nil {
	//	log.Fatal(err)
	//}
	//pprof.StartCPUProfile(cpuProf)
	//
	//defer pprof.StopCPUProfile()
	//
	sess := s3Session()

	cfg := parseConfigFromEnvironment()

	if os.Getenv("_HANDLER") != "" {
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

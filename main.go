package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/spf13/afero"
)

var (
	SinglePageIdentifier = "singlepage"
	MultiPageIdentifier  = "multipage"
)

type Config struct {
	Bucket               string
	ObjectPrefix         string
	TemplateBucketURL    *url.URL
	StaticBucketURL      *url.URL
	IndexType            string
	IndexTemplate        string
	ServerSideEncryption string
	LocalOutputDirectory string
}

func parseConfigFromEnvironment() Config {
	var cfg Config

	var ok bool

	cfg.Bucket, _ = os.LookupEnv("BUCKET")
	cfg.ObjectPrefix, _ = os.LookupEnv("OBJECT_PREFIX")

	if cfg.IndexType, ok = os.LookupEnv("INDEX_TYPE"); !ok {
		cfg.IndexType = MultiPageIdentifier
	} else {
		if cfg.IndexType != MultiPageIdentifier && cfg.IndexType != SinglePageIdentifier {
			log.Fatalf("err: expected multipage or singlepage, found %v", cfg.IndexType)
		}
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

func HandleRequest(sess *session.Session, cfg Config) func(ctx context.Context, event events.S3Event) error {
	return func(ctx context.Context, event events.S3Event) error {
		//lc, _ := lambdacontext.FromContext(ctx)
		fmt.Printf("bucket: %v, key: %v, event: %v",
			event.Records[0].S3.Bucket.Name,
			event.Records[0].S3.Object.Key,
			event.Records[0].EventName,
		)

		outputFS := NewS3OutputFS(sess, cfg.Bucket, &cfg.ServerSideEncryption)

		err := indexS3Bucket(sess, cfg, outputFS)
		if err != nil {
			return err
		}

		return err
	}
}

func indexS3Bucket(sess *session.Session, cfg Config, outputFS afero.Fs) error {
	tmpl, err := LoadTemplates(sess, cfg.TemplateBucketURL)
	if err != nil {
		return fmt.Errorf("failed to load templates: %w", err)
	}

	err = CopyStaticFiles(sess, outputFS, cfg.StaticBucketURL)
	if err != nil {
		return fmt.Errorf("failed to copy static files: %w", err)
	}

	s3Bucket := NewS3Bucket(sess, cfg.Bucket, cfg.ServerSideEncryption)

	var objectTree *ObjectTree
	duration, err := TimeFunc(func() error {
		objectTree, err = CreateObjectTree(s3Bucket, cfg.ObjectPrefix)
		return err
	})
	log.Printf("CreateObjectTree: duration:%v\n", duration)
	if err != nil {
		return fmt.Errorf("failed to create object tree: %w", err)
	}

	renderers := IndexRenderers{
		HTMLIndexRenderer(tmpl, cfg.IndexTemplate),
		JSONIndexRenderer(DioadIndexConfig),
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

	log.Printf("GenerateIndexFiles: duration:%v\n", duration)
	return err
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
	sess := s3Session()

	cfg := parseConfigFromEnvironment()

	if os.Getenv("_HANDLER") != "" {
		lambda.Start(HandleRequest(sess, cfg))
	} else {
		if len(os.Args) >= 2 {
			cfg.Bucket = os.Args[1]

			outputFS, err := localOutputFS(os.Args)
			if err != nil {
				log.Fatalf("failed to create local output FS: %v", err)
			}

			if outputFS == nil {
				outputFS = NewS3OutputFS(sess, cfg.Bucket, &cfg.ServerSideEncryption)
			}

			err = indexS3Bucket(sess, cfg, outputFS)
			if err != nil {
				log.Fatalf("failed to generate index files: %v", err)
			}
		}
	}
}

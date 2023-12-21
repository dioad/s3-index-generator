package main

import (
	"context"
	"errors"
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
	IndexFile            = "index.html"
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

func HandleRequest(ctx context.Context, event events.S3Event) error {
	sess := s3Session()

	// lc, _ := lambdacontext.FromContext(ctx)
	fmt.Printf("bucket: %v, key: %v, event: %v",
		event.Records[0].S3.Bucket.Name,
		event.Records[0].S3.Object.Key,
		event.Records[0].EventName,
	)

	// We should figure bucket out from event
	cfg := parseConfigFromEnvironment()
	if cfg.Bucket == "" {
		return errors.New("no BUCKET environment variable specified")
	}

	outputFS := NewS3OutputFS(sess, cfg.Bucket, &cfg.ServerSideEncryption)

	err := indexS3Bucket(sess, cfg, outputFS)
	if err != nil {
		return err
	}

	return err
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

	objectTree, err := CreateObjectTree(s3Bucket, cfg.ObjectPrefix)
	if err != nil {
		return fmt.Errorf("failed to create object tree: %w", err)
	}

	duration, err := TimeFunc(func() error { return GenerateIndexFiles(objectTree, outputFS, tmpl, cfg.IndexTemplate, cfg.IndexType) })
	log.Printf("GenerateIndexFiles: duration:%v\n", duration)
	return err
}

func TimeFunc(f func() error) (time.Duration, error) {
	start := time.Now()
	err := f()
	return time.Since(start), err
}

func main() {

	if os.Getenv("_HANDLER") != "" {
		lambda.Start(HandleRequest)
	} else {
		if len(os.Args) >= 2 {
			sess := s3Session()

			cfg := parseConfigFromEnvironment()
			cfg.Bucket = os.Args[1]

			var outputFS afero.Fs
			var err error
			if len(os.Args) == 3 {
				cfg.LocalOutputDirectory = os.Args[2]
				outputFS, err = NewLocalOutputFS(cfg.LocalOutputDirectory)
				if err != nil {
					log.Fatalf("failed to create local output FS: %w", err)
				}
			} else {
				outputFS = NewS3OutputFS(sess, cfg.Bucket, &cfg.ServerSideEncryption)
			}

			err = indexS3Bucket(sess, cfg, outputFS)
			if err != nil {
				log.Fatalf("failed to generate index files: %w", err)
			}
		}
	}
}

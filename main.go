package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type Config struct {
	TemplateBucketURL    *url.URL
	IndexType            string
	IndexTemplate        string
	LocalOutputDirectory string
}

func parseConfigFromEnvironment() Config {
	var cfg Config

	var ok bool

	if cfg.IndexType, ok = os.LookupEnv("INDEX_TYPE"); !ok {
		cfg.IndexType = "multipage"
	} else {
		if cfg.IndexType != "multipage" && cfg.IndexType != "singlepage" {
			log.Fatalf("err: expected multipage or singlepage, found %v", cfg.IndexType)
		}
	}

	if cfg.IndexTemplate, ok = os.LookupEnv("INDEX_TEMPLATE"); !ok {
		cfg.IndexTemplate = fmt.Sprintf("%v.index.html.tmpl", cfg.IndexType)
	}

	var templateBucketURLString string

	if templateBucketURLString, ok = os.LookupEnv("TEMPLATE_BUCKET_URL"); ok {
		tmpURL, err := url.Parse(templateBucketURLString)
		if err != nil {
			log.Fatalf("err: unable to parse TEMPLATE_BUCKET_URL as URL: %v", err)
		}
		cfg.TemplateBucketURL = tmpURL
	}

	return cfg
}

func HandleRequest(ctx context.Context, s3Event events.S3Event) error {
	cfg := parseConfigFromEnvironment()
	for _, record := range s3Event.Records {
		if !strings.HasSuffix(record.S3.Object.Key, "index.html") {
			bucketName := record.S3.Bucket.Name
			err := GenerateIndexFiles(cfg, bucketName)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func main() {
	if os.Getenv("_HANDLER") != "" {
		lambda.Start(HandleRequest)
	} else {
		if len(os.Args) >= 2 {
			cfg := parseConfigFromEnvironment()
			if len(os.Args) == 3 {
				cfg.LocalOutputDirectory = os.Args[2]
			}
			err := GenerateIndexFiles(cfg, os.Args[1])
			if err != nil {
				fmt.Printf("err: %v\n", err)
			}
		}
	}
}

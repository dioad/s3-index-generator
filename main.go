package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

var (
	SinglePageIdentifier = "singlepage"
	MultiPageIdentifier  = "multipage"
	IndexFile            = "index.html"
)

type Config struct {
	TemplateBucketURL    *url.URL
	IndexType            string
	IndexTemplate        string
	LocalOutputDirectory string
	ServerSideEncryption string
}

func parseConfigFromEnvironment() Config {
	var cfg Config

	var ok bool

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

	// Can we figure these details out by looking at bucket config?
	cfg.ServerSideEncryption, _ = os.LookupEnv("SSE")

	return cfg
}

func HandleRequest(ctx context.Context, s3Event events.S3Event) error {
	//	lc, _ := lambdacontext.FromContext(ctx)

	eventJson, _ := json.MarshalIndent(s3Event, "", "  ")
	log.Printf("DEBUG/S3-EVENT: %v", eventJson)

	cfg := parseConfigFromEnvironment()
	for _, record := range s3Event.Records {
		key := record.S3.Object.Key
		if !strings.HasSuffix(key, IndexFile) {
			if !strings.HasSuffix(key, "/") {
				log.Printf("DEBUG/OBJ-KEY: %v", key)
				bucketName := record.S3.Bucket.Name
				err := GenerateIndexFiles(cfg, bucketName)
				if err != nil {
					return err
				}
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

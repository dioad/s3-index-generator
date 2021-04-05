package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

var (
	SinglePageIdentifier = "singlepage"
	MultiPageIdentifier  = "multipage"
	IndexFile            = "index.html"
)

type Config struct {
	Bucket               string
	TemplateBucketURL    *url.URL
	IndexType            string
	IndexTemplate        string
	LocalOutputDirectory string
	ServerSideEncryption string
}

func parseConfigFromEnvironment() Config {
	var cfg Config

	var ok bool

	cfg.Bucket, _ = os.LookupEnv("BUCKET")

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

func HandleRequest(ctx context.Context, event events.CloudWatchEvent) error {
	//	lc, _ := lambdacontext.FromContext(ctx)
	eventJson, _ := json.MarshalIndent(event, "", "  ")
	log.Printf("%s", eventJson)

	cfg := parseConfigFromEnvironment()
	if cfg.Bucket == "" {
		return errors.New("no BUCKET environment variable specified")
	}

	return GenerateIndexFiles(cfg, cfg.Bucket)
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

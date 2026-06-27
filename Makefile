GORELEASER ?= goreleaser
GORELEASER_CONFIG ?= .goreleaser.yml
GORELEASER_SNAPSHOT_ARGS ?= release --clean --snapshot --skip=publish --skip=announce --skip=sign

.PHONY: help goreleaser

.DEFAULT_GOAL := help

build:
	go generate ./...
	go build .

format:
	go fmt ./...
	go fix ./...

lint:
	go vet ./...
	golangci-lint run ./...

test:
	go test -race ./...

verify: format lint build

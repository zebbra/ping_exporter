.PHONY: build clean test fmt vet run docker-build docker-push release-snapshot release goreleaser-check

BINARY_NAME=ping_exporter
BINARY_PATH=dist/$(BINARY_NAME)
GO_FILES=$(shell find . -name "*.go" -type f)
VERSION ?= $(shell git describe --tags --always --dirty)
REGISTRY ?= quay.io
IMAGE_NAME ?= zebbra/ping_exporter

all: fmt vet build

build: dist
	go build -o $(BINARY_PATH) .

dist:
	mkdir -p dist

clean:
	go clean
	rm -rf dist
	rm -f $(BINARY_NAME)

test:
	go test -v ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

run: build
	./$(BINARY_PATH)

install:
	go install .

deps:
	go mod download
	go mod tidy

docker-build:
	docker build -t ping_exporter .

docker-push:
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) \
		-t $(REGISTRY)/$(IMAGE_NAME):latest \
		--push .

goreleaser-check:
	goreleaser check

release-snapshot:
	goreleaser release --clean --snapshot

release:
	goreleaser release --clean

release-dry-run:
	goreleaser release --skip-publish --clean

.DEFAULT_GOAL := all
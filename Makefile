.PHONY: build clean test fmt vet run

BINARY_NAME=ping_exporter
BINARY_PATH=dist/$(BINARY_NAME)
GO_FILES=$(shell find . -name "*.go" -type f)

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

.DEFAULT_GOAL := all
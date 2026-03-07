APP_NAME := gostratumengine
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -X main.version=$(VERSION) -X main.buildDate=$(BUILD_DATE) -X main.commit=$(COMMIT)

.PHONY: build build-all clean test lint

## build: Build for current platform
build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(APP_NAME) ./cmd/gostratumengine/

## build-linux-amd64: Build for Linux AMD64
build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" \
		-o bin/$(APP_NAME)-linux-amd64 ./cmd/gostratumengine/

## build-linux-arm64: Build for Linux ARM64
build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" \
		-o bin/$(APP_NAME)-linux-arm64 ./cmd/gostratumengine/

## build-darwin-amd64: Build for macOS AMD64
build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" \
		-o bin/$(APP_NAME)-darwin-amd64 ./cmd/gostratumengine/

## build-darwin-arm64: Build for macOS ARM64
build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" \
		-o bin/$(APP_NAME)-darwin-arm64 ./cmd/gostratumengine/

## build-all: Build for all supported platforms
build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64

## test: Run all tests
test:
	go test -v ./...

## test-short: Run tests without verbose output
test-short:
	go test ./...

## lint: Run go vet
lint:
	go vet ./...

## clean: Remove build artifacts
clean:
	rm -rf bin/

## help: Show this help
help:
	@echo "GoStratumEngine - Open Source Stratum V1 Engine"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

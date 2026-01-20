# build go files
DIR_BIN=./bin
AMD=amd64
ARM=arm64

# Informações de versão
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Flags de build para injetar informações de versão
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildDate=$(BUILD_DATE) -X main.gitCommit=$(GIT_COMMIT)"

.PHONY: help
help:
	@echo "Makefile commands:"
	@echo "  make build    - Build the Go binaries for amd64 and arm64 architectures"


build:
	@GOOS=linux GOARCH=$(AMD) go build $(LDFLAGS) -o bin/$(AMD)/sc .
	@chmod +x bin/$(AMD)/sc
	@GOOS=darwin GOARCH=$(ARM) go build $(LDFLAGS) -o bin/$(ARM)/sc .
	@chmod +x bin/$(ARM)/sc
	@echo "Build completed: binaries are in $(DIR_BIN)/$(AMD)/sc and $(DIR_BIN)/$(ARM)/sc"
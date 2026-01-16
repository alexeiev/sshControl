# build go files
DIR_BIN=./bin
AMD=amd64
ARM=arm64

.PHONY: help
help:
	@echo "Makefile commands:"
	@echo "  make build    - Build the Go binaries for amd64 and arm64 architectures"


build:
	@GOOS=linux GOARCH=$(AMD) go build -o bin/$(AMD)/sc .
	@chmod +x bin/$(AMD)/sc 
	@GOOS=darwin GOARCH=$(ARM) go build -o bin/$(ARM)/sc .
	@chmod +x bin/$(ARM)/sc 
	@echo "Build completed: binaries are in $(DIR_BIN)/$(AMD)/sc and $(DIR_BIN)/$(ARM)/sc"
.PHONY: build clean install test

BINARY=virtuoso
OUTPUT_DIR=bin

build:
	go build -o $(OUTPUT_DIR)/$(BINARY) ./cmd/virtuoso

clean:
	rm -rf $(OUTPUT_DIR)

install:
	go install ./cmd/virtuoso

test:
	go test ./...

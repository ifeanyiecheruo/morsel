.PHONY: build test lint run-local ci

build:
	go build -o bin/morsel ./cmd/morsel

test:
	go test ./...

lint:
	go vet ./...

run:
	go run ./cmd/morsel --platform local

ci: lint build test

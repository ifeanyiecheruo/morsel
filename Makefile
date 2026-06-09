.PHONY: build test lint run ci

build:
	go build -o bin/morsel ./cmd/morsel
	go build -o bin/morsel-api ./cmd/morsel-api

test:
	go test ./...

lint:
	go vet ./...

run:
	go run ./cmd/morsel --platform local

ci: lint build test

.PHONY: build test lint run ci help install-tools
.DEFAULT_GOAL := help

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-8s %s\n", $$1, $$2}'

build: ## Compile bin/morsel and bin/morsel-api
	go build -o bin/morsel ./cmd/morsel
	go build -o bin/morsel-api ./cmd/morsel-api

test: ## Run all tests
	go test ./...

lint: ## Run go vet
	go vet ./...

run: ## Run the CLI against LocalPlatform
	go run ./cmd/morsel --platform local

ci: lint build test ## Lint, build, and test (what CI runs)

install-tools: ## Install go and git if not already installed
	@if ! command -v git >/dev/null 2>&1; then \
		echo "Installing git..."; \
		if [ "$$(uname)" = "Darwin" ]; then brew install git; \
		elif command -v apt-get >/dev/null 2>&1; then sudo apt-get install -y git; \
		elif command -v yum >/dev/null 2>&1; then sudo yum install -y git; \
		else echo "Please install git manually: https://git-scm.com/" && exit 1; fi; \
	else echo "git: already installed"; fi
	@if ! command -v go >/dev/null 2>&1; then \
		echo "Installing go..."; \
		if [ "$$(uname)" = "Darwin" ]; then brew install go; \
		elif command -v apt-get >/dev/null 2>&1; then sudo apt-get install -y golang-go; \
		elif command -v yum >/dev/null 2>&1; then sudo yum install -y golang; \
		else echo "Please install Go manually: https://go.dev/dl/" && exit 1; fi; \
	else echo "go: already installed"; fi

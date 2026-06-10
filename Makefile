.DEFAULT_GOAL := help

# ---- Local Go ecosystem --------------------------------------------------------
# REPO_ROOT uses pwd -W (real Windows path C:/...) for Go env vars that need
# Windows-style paths. LOCAL_SHELL uses cygpath -u to get the MSYS-style path
# (/c/...) for PATH — a Windows drive letter in PATH would split on the colon
# and make the directory unreachable. On Linux/macOS cygpath is absent so both
# fall back to the same plain pwd output.
REPO_ROOT   := $(shell pwd -W 2>/dev/null || pwd)
LOCAL       := $(REPO_ROOT)/.local
LOCAL_SHELL := $(shell cygpath -u "$(REPO_ROOT)" 2>/dev/null || echo "$(REPO_ROOT)")/.local
GO_VERSION  := $(shell sed -n 's/^go //p' go.mod)
export PATH     := $(LOCAL_SHELL)/go/bin:$(LOCAL_SHELL)/bin:$(PATH)
export GOPATH   := $(LOCAL)
export GOCACHE             := $(LOCAL)/cache
export GOLANGCI_LINT_CACHE := $(LOCAL)/golangci-lint-cache
export GOTMPDIR            := $(LOCAL)/tmp
export TMP                 := $(LOCAL)/tmp
export TEMP                := $(LOCAL)/tmp
$(shell mkdir -p "$(LOCAL_SHELL)/cache" "$(LOCAL_SHELL)/golangci-lint-cache" "$(LOCAL_SHELL)/tmp")
# --------------------------------------------------------------------------------

.PHONY: help
help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-16s %s\n", $$1, $$2}' 2>/dev/null; true

.PHONY: build
build: ## Compile bin/morsel and bin/morsel-api
	go build -o bin/morsel ./cmd/morsel
	go build -o bin/morsel-api ./cmd/morsel-api

.PHONY: test
test: ## Run all tests
	go test ./...

.PHONY: lint
lint: ## Run linters
	go vet ./...
	golangci-lint run
	@UNFORMATTED=$$(go list -f '{{.Dir}}' ./... | sed 's|\\|/|g' | xargs gofmt -l); \
	test -z "$$UNFORMATTED" || { echo "Files need formatting (run make fix):"; echo "$$UNFORMATTED"; exit 1; }

.PHONY: fix
fix: ## Auto-fix all fixable issues
	go mod tidy
	golangci-lint run --fix
	go fmt ./...

.PHONY: run
run: ## Run the CLI against LocalPlatform
	go run ./cmd/morsel --platform local

.PHONY: ci
ci: generate-ci lint build test ## what CI runs

.PHONY: generate
generate: ## Regenerate go code
	go generate ./...

.PHONY: generate-ci
generate-ci:
	go generate ./...
	git diff --exit-code -- internal/db/queries/

.PHONY: pre-commit
pre-commit: generate fix ## Pre-commit git hook	
	@git add -u

.PHONY: pre-push
pre-push: ci ## Pre-push git hook

.PHONY: configure
configure: ## Configure repository (installs git hooks, etc.)
	@echo '#!/bin/sh' > .git/hooks/pre-commit
	@echo 'make pre-commit' >> .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo '#!/bin/sh' > .git/hooks/pre-push
	@echo 'make pre-push' >> .git/hooks/pre-push
	@chmod +x .git/hooks/pre-push
	@echo "Git hooks installed."

.PHONY: install-tools
install-tools: ## Install prerequisites
	@mkdir -p $(LOCAL)/tmp
	@if [ -x "$(LOCAL)/go/bin/go" ]; then \
		echo "go: already installed at $(LOCAL)/go ($$($(LOCAL)/go/bin/go version))"; \
	elif command -v go >/dev/null 2>&1; then \
		echo "go: found in PATH ($$(go version)), skipping local install"; \
	else \
		OS=$$(uname -s | tr '[:upper:]' '[:lower:]' | sed 's/mingw.*/windows/;s/msys.*/windows/;s/cygwin.*/windows/'); \
		ARCH=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/'); \
		echo "Downloading Go $(GO_VERSION) for $$OS/$$ARCH into $(LOCAL) ..."; \
		if [ "$$OS" = "windows" ]; then \
			curl -fsSL "https://go.dev/dl/go$(GO_VERSION).$$OS-$$ARCH.zip" -o "$(LOCAL)/go.zip"; \
			WIN_LOCAL=$$(cygpath -m "$(LOCAL)"); \
			powershell.exe -NoProfile -Command "Add-Type -Assembly 'System.IO.Compression.FileSystem'; [System.IO.Compression.ZipFile]::ExtractToDirectory('$$WIN_LOCAL/go.zip', '$$WIN_LOCAL')"; \
			rm "$(LOCAL)/go.zip"; \
		else \
			curl -fsSL "https://go.dev/dl/go$(GO_VERSION).$$OS-$$ARCH.tar.gz" | tar -C "$(LOCAL)" -xz; \
		fi; \
	fi
	@if ! command -v git >/dev/null 2>&1; then \
		echo "git not found; please install it: https://git-scm.com/"; exit 1; \
	else echo "git: found ($$(git --version))"; fi
	@echo "Installing tool binaries from go.mod into $(LOCAL)/bin ..."
	@awk '/^tool [^(]/{print $$2} /^tool \(/{f=1;next} f&&/^\)/{f=0} f&&NF{print $$1}' go.mod | \
	while read -r tool; do echo "  $$tool"; go install "$$tool"; done

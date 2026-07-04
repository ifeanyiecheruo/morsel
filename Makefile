.DEFAULT_GOAL := help

# Detect Windows regardless of which POSIX shell drives make.
# Native Windows (cmd/PS) sets OS=Windows_NT; MSYS2/MINGW environments
# (including devkitPro) set MSYSTEM but may strip OS from the environment.
_is_windows := $(or $(filter Windows_NT,$(OS)),$(MSYSTEM))

ifneq ($(_is_windows),)
EXE := .exe
else
EXE :=
endif

CLUSTER_NAME  ?= morsel-local
K3D_VERSION   ?= v5.7.5

# ---- Container runtime --------------------------------------------------------
# docker takes precedence when its daemon is reachable; otherwise fall back to
# rootless podman. k3d handles podman via DOCKER_HOST set at runtime.
_docker_ok := $(shell docker info >/dev/null 2>&1 && echo yes)
ifeq ($(_docker_ok),yes)
CONTAINER_RUNTIME := docker
else
CONTAINER_RUNTIME := podman
endif
# -------------------------------------------------------------------------------

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
# Append the default winget podman install location so it is findable
# immediately after make install-tools without reopening the shell.
ifneq ($(_is_windows),)
export PATH     := $(PATH):/c/Program Files/RedHat/Podman
# devkitPro MSYS2 (and other POSIX shells) omit USERPROFILE, APPDATA, and
# LOCALAPPDATA from their environment. Native Windows programs (podman, docker)
# need these to locate user config. Use PowerShell's Windows API to read the
# real profile path, bypassing environment variable inheritance entirely.
_win_userprofile := $(shell /c/Program\ Files/PowerShell/7/pwsh.exe -NoProfile -Command "Write-Host -NoNewline ([Environment]::GetFolderPath('UserProfile'))")
export USERPROFILE  := $(_win_userprofile)
export APPDATA      := $(_win_userprofile)\AppData\Roaming
export LOCALAPPDATA := $(_win_userprofile)\AppData\Local
endif
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

build: bin/morsel$(EXE) bin/morsel-ctrl-plane$(EXE)

.PHONY: deploy
deploy: build ## Deploy the CLI against LocalPlatform
	bin/morsel$(EXE) service deploy --yes --platform local --force
	bin/morsel$(EXE) app deploy

.PHONY: deploy-apps
deploy-apps: deploy ## Deploy morsel apps against LocalPlatform
	bin/morsel$(EXE) app deploy

.PHONY: test
test: ## Run all tests
	go test ./...

.PHONY: lint
lint: ## Run linters
	go vet ./...
	golangci-lint run
	@UNFORMATTED=$$(gofmt -l $$(go list -f '{{.Dir}}' ./... | sed 's|\\|/|g')); \
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
	git diff --exit-code -- .

.PHONY: pre-commit
pre-commit: ## Pre-commit git hook
	@git stash --keep-index --quiet || true; \
	$(MAKE) generate fix && git add -u; \
	git stash pop --quiet || true

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
	@if command -v k3d >/dev/null 2>&1; then \
		echo "k3d: found ($$(k3d version | head -1))"; \
	else \
		echo "Installing k3d $(K3D_VERSION) ..."; \
		go install github.com/k3d-io/k3d/v5@$(K3D_VERSION); \
	fi
	@if ! docker info >/dev/null 2>&1 && ! command -v podman >/dev/null 2>&1; then \
		echo "Installing podman (no docker daemon found) ..."; \
		if [ "$$OS" = "windows" ]; then \
			winget install --id RedHat.Podman -e --accept-source-agreements --accept-package-agreements; \
		elif uname -s | grep -qi darwin; then \
			brew install podman; \
		elif command -v apt-get >/dev/null 2>&1; then \
			sudo apt-get install -y podman; \
		elif command -v dnf >/dev/null 2>&1; then \
			sudo dnf install -y podman; \
		else \
			echo "Cannot auto-install podman; see https://podman.io/docs/installation"; exit 1; \
		fi; \
	else \
		echo "container runtime: $(CONTAINER_RUNTIME)"; \
	fi

.PHONY: cluster-down
cluster-down: ## Delete the local k3d cluster (override: CLUSTER_NAME=morsel-dev)
	k3d cluster delete $(CLUSTER_NAME)

# _ensure-podman-machine starts the default podman machine on Windows/macOS
# when podman is the container runtime. No-op on Linux (native socket).
.PHONY: _ensure-podman-machine
_ensure-podman-machine:
ifeq ($(CONTAINER_RUNTIME),podman)
	@if ! uname -s | grep -qi linux; then \
		podman machine inspect >/dev/null 2>&1 || podman machine init; \
		podman info >/dev/null 2>&1 || podman machine start; \
	fi
endif

bin/morsel$(EXE): $(shell find cmd/morsel internal -name '*.go')
	go build -o bin/morsel$(EXE) ./cmd/morsel

bin/morsel-ctrl-plane$(EXE): $(shell find cmd/morsel-ctrl-plane internal -name '*.go')
	go build -o bin/morsel-ctrl-plane$(EXE) ./cmd/morsel-ctrl-plane


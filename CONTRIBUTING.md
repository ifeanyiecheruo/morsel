# Contributing to Morsel

## Prerequisites

- A local Kubernetes cluster for local testing (e.g. Docker Desktop, kind, or k3s)

## Getting started

```sh
git clone https://github.com/ifeanyiecheruo/morsel
cd morsel
make install-tools   # install go and git if not already installed
make ci              # lint, build, test
make run             # run the CLI against LocalPlatform
```

Binaries land in `bin/`. Run `make help` for the full list of targets. The test suite has no external dependencies and runs entirely in-process.

Read [docs/specs/README.md](docs/specs/README.md) before making architectural changes.

## Code conventions

See [docs/contributing/conventions.md](docs/contributing/conventions.md).

## Tests

See [docs/contributing/testing.md](docs/contributing/testing.md).

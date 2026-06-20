//go:generate go run ../../cmd/bundlespec --in openapi/openapi.yaml --out openapi/openapi.json
//go:generate go run github.com/ogen-go/ogen/cmd/ogen --config ../../.ogen.yml --target oas --package oas --clean openapi/openapi.yaml

package api

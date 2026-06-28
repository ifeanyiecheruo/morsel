package oas

// Generates and embeds a flattened OpenAPI JSON spec.

//go:generate go run ../../../../../cmd/openapi-bundler --in openapi.yaml --out openapi.json

import _ "embed"

//go:embed openapi.json
var SpecJSON []byte

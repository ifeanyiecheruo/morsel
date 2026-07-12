package oas

// Generates and embeds a flattened OpenAPI JSON spec.

//go:generate go-deps gen bundle.gen.json

import _ "embed"

//go:embed openapi.json
var SpecJSON []byte

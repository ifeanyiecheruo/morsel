// Package openapi embeds the pre-generated flattened OpenAPI JSON spec.
package openapi

import _ "embed"

//go:embed openapi.json
var SpecJSON []byte

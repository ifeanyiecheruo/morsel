// Package openapi embeds the OpenAPI specification source files so they can
// be bundled into a single resolved document at runtime.
package openapi

import "embed"

//go:embed openapi.yaml parameters.yaml responses.yaml
//go:embed schemas
//go:embed paths
var FS embed.FS

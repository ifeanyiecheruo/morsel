// Package schemas embeds the published Morsel JSON schemas for use by the
// lint tool and any other consumers within the module.
package schemas

import _ "embed"

//go:embed morsel.schema.json
var MorselSchemaJSON []byte

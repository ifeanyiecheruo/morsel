// Package platforms resolves a platform name to a platform.Platform implementation.
// It lives in internal/ rather than platform/ to avoid the import cycle that would
// arise from platform/ importing platform/local (which itself imports platform/).
package platforms

import (
	"fmt"

	"github.com/ifeanyiecheruo/morsel/platform"
	"github.com/ifeanyiecheruo/morsel/platform/local"
)

// Create constructs the Platform implementation for the given name.
// An empty name defaults to "local".
func Create(name string) (platform.Platform, error) {
	switch name {
	case "local", "":
		return local.New(), nil
	default:
		return nil, fmt.Errorf("unknown platform %q (supported: local)", name)
	}
}

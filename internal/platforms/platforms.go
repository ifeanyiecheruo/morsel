// Package platforms resolves a platform name to a platform.Platform implementation.
// It lives in internal/ rather than platform/ to avoid the import cycle that would
// arise from platform/ importing platform/local (which itself imports platform/).
package platforms

import (
	"fmt"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/platform/local"
	"github.com/ifeanyiecheruo/morsel/internal/store"
)

// Create constructs the Platform implementation for the given name.
// Pass a non-nil store for server contexts that need principal validation;
// pass nil for CLI contexts.
func Create(name string, s *store.Store) (platform.Platform, error) {
	switch name {
	case "local":
		return local.New(s), nil
	default:
		return nil, fmt.Errorf("unknown platform %q (supported: local)", name)
	}
}

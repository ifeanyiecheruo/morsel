// Package platforms resolves a platform name to its Bootstrapper implementation.
package platforms

import (
	"fmt"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/platform/local"
)

// New returns the Bootstrapper for the given platform name.
func New(platformName string) (platform.Bootstrapper, error) {
	switch platformName {
	case "local":
		return local.New(), nil
	default:
		return nil, fmt.Errorf("unknown platform %q (supported: local)", platformName)
	}
}

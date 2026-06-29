// Package platforms resolves a platform name to its ServiceDeployer implementation.
package platforms

import (
	"fmt"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/platform/local"
)

// New returns the ServiceDeployer for the given platform name.
func New(platformName string) (platform.ServiceDeployer, error) {
	switch platformName {
	case "local":
		return local.New(), nil
	default:
		return nil, fmt.Errorf("unknown platform %q (supported: local)", platformName)
	}
}

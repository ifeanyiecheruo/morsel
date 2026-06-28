// Package localboot provides the bootstrap implementation for the local platform.
// It is shared between the CLI (which runs bootstrap) and the control plane
// (which needs the LocalBaseDomain and cert-generation helpers).
package localboot

import (
	"fmt"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

// New returns a Bootstrapper for the given platform name.
func New(platformName string) (platform.Bootstrapper, error) {
	switch platformName {
	case "local":
		return &localBootstrapper{}, nil
	default:
		return nil, fmt.Errorf("unknown platform %q (supported: local)", platformName)
	}
}

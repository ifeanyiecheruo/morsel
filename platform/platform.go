// Package platform exports the CLI-facing subset of the Morsel platform
// interface. The full server-facing interface lives in internal/platform
// and is not visible outside this module.
package platform

import intplat "github.com/ifeanyiecheruo/morsel/internal/platform"

// Platform is the CLI-facing subset: Bootstrap and Deploy only.
// The REST API server consumes internal/platform.Platform directly.
type (
	Platform          = intplat.CliPlatform
	Bootstrapper      = intplat.Bootstrapper
	Deployer          = intplat.Deployer
	Prompt            = intplat.Prompt
	Plan              = intplat.Plan
	Resource          = intplat.Resource
	DeployCredentials = intplat.DeployCredentials
)

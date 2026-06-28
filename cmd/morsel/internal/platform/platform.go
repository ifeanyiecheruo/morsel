// Package platform defines the CLI-facing platform interface.
// All platforms must implement Bootstrapper to support the service bootstrap command.
package platform

import "context"

// Bootstrapper provisions all platform resources needed to run Morsel.
type Bootstrapper interface {
	CheckPrerequisites(ctx context.Context, kubeconfig string, answers map[string]string) error
	KubeconfigPath() string
	KubeContext() string
	ClusterServer() string
	APIURL() string
	Prompts() []Prompt
	Plan(answers map[string]string) Plan
	Provision(ctx context.Context, answers map[string]string, dockerfile []byte) error
}

// Prompt is a single wizard question presented to the operator during bootstrap.
type Prompt struct {
	Key         string
	Label       string
	Description string
	Default     string
	Required    bool
	Secret      bool
	Choices     []string
	Validate    func(string) error
}

// Plan describes what will be created and estimated costs before operator confirmation.
type Plan struct {
	Summary   string
	Resources []Resource
}

// Resource is one provisioned item in a bootstrap Plan.
type Resource struct {
	Name            string
	Description     string
	EstimatedCostMo float64
}

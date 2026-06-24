// Package kube provides the Kubernetes client used by the Morsel API to apply
// app manifests and watch rollout status. It is the only place in the codebase
// that imports client-go.
package kube

import (
	"errors"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ConfigError is returned by New when a kubeconfig cannot be resolved or parsed.
// Callers can inspect KubeconfigPath to determine whether an explicit path was
// requested (non-empty) or default resolution was used (empty).
type ConfigError struct {
	KubeconfigPath string // empty when default resolution order was used
	Err            error
}

func (e *ConfigError) Error() string { return e.Err.Error() }
func (e *ConfigError) Unwrap() error { return e.Err }

// Client wraps the Kubernetes Clientset with Morsel-specific manifest apply
// and status query operations.
type Client struct {
	cs kubernetes.Interface
}

// New creates a Client. It tries in-cluster config first (service account
// mounted at /var/run/secrets), then falls back to the kubeconfig at
// kubeconfigPath. If kubeconfigPath is empty it uses the standard kubeconfig
// resolution order ($KUBECONFIG, then ~/.kube/config).
func New(kubeconfigPath string) (*Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		if kubeconfigPath != "" {
			loadingRules.ExplicitPath = kubeconfigPath
		}
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules, nil,
		).ClientConfig()
		if err != nil {
			return nil, &ConfigError{
				KubeconfigPath: kubeconfigPath,
				Err:            fmt.Errorf("resolve kubernetes config: %w", err),
			}
		}
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build kubernetes client: %w", err)
	}
	return &Client{cs: cs}, nil
}

// NewFromClientset wraps an existing clientset. Intended for tests.
func NewFromClientset(cs kubernetes.Interface) *Client {
	return &Client{cs: cs}
}

// IsConfigError reports whether err (or any error it wraps) is a ConfigError.
func IsConfigError(err error) bool {
	return errors.As(err, new(*ConfigError))
}

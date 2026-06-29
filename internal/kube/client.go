// Package kube provides the Kubernetes client used by the Morsel API to apply
// app manifests and watch rollout status. It is the only place in the codebase
// that imports client-go.
package kube

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
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
	cs  kubernetes.Interface
	gw  gatewayclient.Interface
	dyn dynamic.Interface
}

// New creates a Client. It tries in-cluster config first (service account
// mounted at /var/run/secrets), then falls back to the kubeconfig at
// kubeconfigPath. If kubeconfigPath is empty it uses the standard kubeconfig
// resolution order ($KUBECONFIG, then ~/.kube/config).
func New(kubeconfigPath string) (*Client, error) {
	cfg, err := buildKubeConfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build kubernetes client: %w", err)
	}
	gw, err := gatewayclient.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build gateway client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}
	return &Client{cs: cs, gw: gw, dyn: dyn}, nil
}

// buildKubeConfig returns a *rest.Config using in-cluster config first, then
// the kubeconfig at kubeconfigPath (or the default resolution order if empty).
func buildKubeConfig(kubeconfigPath string) (*rest.Config, error) {
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
	return cfg, nil
}

// NewFromClientset wraps an existing clientset. Intended for tests.
func NewFromClientset(cs kubernetes.Interface) *Client {
	return &Client{cs: cs}
}

// IsConfigError reports whether err (or any error it wraps) is a ConfigError.
func IsConfigError(err error) bool {
	return errors.As(err, new(*ConfigError))
}

// marshalPrivateKey encodes any private key to PKCS#8 PEM ("PRIVATE KEY").
func marshalPrivateKey(key any) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

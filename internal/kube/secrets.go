package kube

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

// ErrSecretNotFound is returned by GetOpaqueSecret when the named secret does not exist.
var ErrSecretNotFound = errors.New("kube: secret not found")

const opaqueSecretDataKey = "value"

// NewInCluster creates a Client using the in-cluster service account config.
// Returns an error when not running inside a Kubernetes pod.
func NewInCluster() (*Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	gw, err := gatewayclient.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{cs: cs, gw: gw}, nil
}

// GetOpaqueSecret reads the raw bytes stored in a morsel opaque K8s Secret.
// Returns ErrSecretNotFound when the secret does not exist.
func (c *Client) GetOpaqueSecret(ctx context.Context, namespace, name string) ([]byte, error) {
	secret, err := c.cs.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, err
	}
	return secret.Data[opaqueSecretDataKey], nil
}

// SetOpaqueSecret creates or updates an opaque K8s Secret, storing data under the "value" key.
// Idempotent — safe to call repeatedly.
func (c *Client) SetOpaqueSecret(ctx context.Context, namespace, name string, data []byte) error {
	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"morsel.io/managed": "true"},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{opaqueSecretDataKey: data},
	}
	existing, err := c.cs.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.CoreV1().Secrets(namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Data = desired.Data
	_, err = c.cs.CoreV1().Secrets(namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// DeleteOpaqueSecret deletes a named opaque K8s Secret. Returns nil if it does not exist.
func (c *Client) DeleteOpaqueSecret(ctx context.Context, namespace, name string) error {
	err := c.cs.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return err
}

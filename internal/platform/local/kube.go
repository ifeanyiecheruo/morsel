package local

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// KubeconfigContext holds the resolved cluster connection parameters for the
// active context in a kubeconfig file.
type KubeconfigContext struct {
	ContextName string
	ServerURL   string
	tlsConfig   *tls.Config
	token       string
}

// kubeconfigFile mirrors the top-level shape of a kubeconfig YAML file.
type kubeconfigFile struct {
	CurrentContext string              `yaml:"current-context"`
	Clusters       []kubeconfigCluster `yaml:"clusters"`
	Users          []kubeconfigUser    `yaml:"users"`
	Contexts       []kubeconfigCtx     `yaml:"contexts"`
}

type kubeconfigCluster struct {
	Name    string `yaml:"name"`
	Cluster struct {
		Server                   string `yaml:"server"`
		CertificateAuthorityData string `yaml:"certificate-authority-data"`
	} `yaml:"cluster"`
}

type kubeconfigUser struct {
	Name string `yaml:"name"`
	User struct {
		ClientCertificateData string `yaml:"client-certificate-data"`
		ClientKeyData         string `yaml:"client-key-data"`
		Token                 string `yaml:"token"`
	} `yaml:"user"`
}

type kubeconfigCtx struct {
	Name    string `yaml:"name"`
	Context struct {
		Cluster string `yaml:"cluster"`
		User    string `yaml:"user"`
	} `yaml:"context"`
}

// DefaultKubeconfigPath returns the kubeconfig path using the same resolution
// order as kubectl: KUBECONFIG env var, then ~/.kube/config.
func DefaultKubeconfigPath() string {
	if v := os.Getenv("KUBECONFIG"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kube", "config")
}

// LoadKubeconfig parses the kubeconfig at path and resolves the active context.
// If contextName is empty the file's current-context is used.
func LoadKubeconfig(path, contextName string) (*KubeconfigContext, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read kubeconfig %q: %w", path, err)
	}
	var kc kubeconfigFile
	if err := yaml.Unmarshal(raw, &kc); err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	if contextName == "" {
		contextName = kc.CurrentContext
	}
	if contextName == "" {
		return nil, fmt.Errorf("kubeconfig has no current-context and none was specified")
	}

	// Resolve context → cluster + user names.
	var clusterName, userName string
	for _, c := range kc.Contexts {
		if c.Name == contextName {
			clusterName = c.Context.Cluster
			userName = c.Context.User
			break
		}
	}
	if clusterName == "" {
		return nil, fmt.Errorf("context %q not found in kubeconfig", contextName)
	}

	// Resolve cluster info.
	var serverURL, caData string
	for _, cl := range kc.Clusters {
		if cl.Name == clusterName {
			serverURL = cl.Cluster.Server
			caData = cl.Cluster.CertificateAuthorityData
			break
		}
	}
	if serverURL == "" {
		return nil, fmt.Errorf("cluster %q not found in kubeconfig", clusterName)
	}

	// Resolve user info.
	var certData, keyData, token string
	for _, u := range kc.Users {
		if u.Name == userName {
			certData = u.User.ClientCertificateData
			keyData = u.User.ClientKeyData
			token = u.User.Token
			break
		}
	}

	tlsCfg, err := buildTLSConfig(caData, certData, keyData)
	if err != nil {
		return nil, fmt.Errorf("build TLS config: %w", err)
	}

	return &KubeconfigContext{
		ContextName: contextName,
		ServerURL:   serverURL,
		tlsConfig:   tlsCfg,
		token:       token,
	}, nil
}

// CheckAccess makes a lightweight authenticated request to the cluster API to
// verify connectivity. Returns nil on success.
func (kc *KubeconfigContext) CheckAccess(ctx context.Context) error {
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: kc.tlsConfig},
		Timeout:   10 * time.Second,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kc.ServerURL+"/api", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if kc.token != "" {
		req.Header.Set("Authorization", "Bearer "+kc.token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to cluster %s: %w", kc.ServerURL, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("cluster returned %d", resp.StatusCode)
	}
	return nil
}

func buildTLSConfig(caData, certData, keyData string) (*tls.Config, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	if caData != "" {
		caRaw, err := base64.StdEncoding.DecodeString(caData)
		if err != nil {
			return nil, fmt.Errorf("decode CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caRaw) {
			return nil, fmt.Errorf("parse CA cert")
		}
		tlsCfg.RootCAs = pool
	}

	if certData != "" && keyData != "" {
		certRaw, err := base64.StdEncoding.DecodeString(certData)
		if err != nil {
			return nil, fmt.Errorf("decode client cert: %w", err)
		}
		keyRaw, err := base64.StdEncoding.DecodeString(keyData)
		if err != nil {
			return nil, fmt.Errorf("decode client key: %w", err)
		}
		cert, err := tls.X509KeyPair(certRaw, keyRaw)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}

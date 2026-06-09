// Package local provides the LocalPlatform implementation of platform.Platform.
// It has no cloud dependencies and runs entirely on the developer's machine.
package local

import "github.com/ifeanyiecheruo/morsel/platform"

// LocalPlatform implements platform.Platform with no cloud dependencies.
type LocalPlatform struct{}

func New() *LocalPlatform { return &LocalPlatform{} }

func (p *LocalPlatform) Bootstrap() platform.Bootstrapper        { return &localBootstrapper{} }
func (p *LocalPlatform) Deploy() platform.Deployer               { return &localDeployer{} }
func (p *LocalPlatform) Blobs() platform.BlobStore               { return &localBlobStore{} }
func (p *LocalPlatform) Secrets() platform.SecretStore           { return &localSecretStore{} }
func (p *LocalPlatform) Credentials() platform.CredentialProvider { return &localCredentialProvider{} }
func (p *LocalPlatform) DNS() platform.DNSProvider               { return &localDNSProvider{} }
func (p *LocalPlatform) Certs() platform.CertProvider            { return &localCertProvider{} }
func (p *LocalPlatform) Pricing() platform.PricingProvider       { return &localPricingProvider{} }

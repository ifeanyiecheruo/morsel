package kube

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	// envoyGatewayNamespace is where the Envoy Gateway controller is installed.
	envoyGatewayNamespace = "envoy-gateway-system"
	// envoyGatewayDeployment is the Deployment name for the Envoy Gateway controller.
	envoyGatewayDeployment = "envoy-gateway"

	// appServiceName is the fixed name for the per-app Kubernetes Service.
	appServiceName = "app"
	// appServicePort is the port the app container listens on.
	appServicePort = int32(8080)
	// httprouteName is the fixed name for the per-app HTTPRoute.
	httprouteName = "app"

	// GatewayClassExternal is the GatewayClass name for public apps.
	GatewayClassExternal = "morsel-external"
	// GatewayClassInternal is the GatewayClass name for private apps.
	GatewayClassInternal = "morsel-internal"
	// GatewayExternal is the name of the external-facing Gateway.
	GatewayExternal = "morsel-external"
	// GatewayInternal is the name of the internal-only Gateway.
	GatewayInternal = "morsel-internal"
	// EnvoyGatewayController is the controller name for Envoy Gateway.
	EnvoyGatewayController = "gateway.envoyproxy.io/gatewayclass-controller"
	// MorselTLSSecret is the name of the K8s Secret holding the platform wildcard TLS cert.
	MorselTLSSecret = "morsel-tls"
)

// resolvePort returns m.Port if set, otherwise the platform default (appServicePort).
func resolvePort(m AppManifest) int32 {
	if m.Port > 0 {
		return m.Port
	}
	return appServicePort
}

// GatewayAPICRDsInstalled reports whether the Gateway API CRDs are registered
// in the cluster. Returns false when the gw client is nil (e.g. in tests).
func (c *Client) GatewayAPICRDsInstalled(ctx context.Context) bool {
	if c.gw == nil {
		return false
	}
	_, err := c.gw.GatewayV1().GatewayClasses().List(ctx, metav1.ListOptions{Limit: 1})
	return err == nil
}

// EnvoyGatewayInstalled reports whether the Envoy Gateway controller is present
// in the cluster by checking for its Deployment in envoy-gateway-system.
func (c *Client) EnvoyGatewayInstalled(ctx context.Context) bool {
	_, err := c.cs.AppsV1().Deployments(envoyGatewayNamespace).Get(ctx, envoyGatewayDeployment, metav1.GetOptions{})
	return err == nil
}

// WaitForEnvoyGatewayReady polls until the Envoy Gateway controller Deployment
// reaches ReadyReplicas ≥ 1 or timeout elapses.
func (c *Client) WaitForEnvoyGatewayReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		deploy, err := c.cs.AppsV1().Deployments(envoyGatewayNamespace).Get(ctx, envoyGatewayDeployment, metav1.GetOptions{})
		if err == nil && deploy.Status.ReadyReplicas >= 1 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("envoy-gateway controller not ready after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// EnsureExternalGateway provisions the external-facing Gateway in ns.
// This is used on the local platform where a single node cannot bind port 443
// twice, making a second internal Gateway impractical.
func (c *Client) EnsureExternalGateway(ctx context.Context, ns, tlsSecretName string) error {
	return c.applyGateway(ctx, ns, GatewayExternal, GatewayClassExternal, tlsSecretName)
}

// EnsureGatewayClasses provisions the external and internal GatewayClass resources.
// Idempotent — safe to call on every bootstrap run.
func (c *Client) EnsureGatewayClasses(ctx context.Context) error {
	if err := c.applyGatewayClass(ctx, GatewayClassExternal, EnvoyGatewayController); err != nil {
		return fmt.Errorf("external gateway class: %w", err)
	}
	if err := c.applyGatewayClass(ctx, GatewayClassInternal, EnvoyGatewayController); err != nil {
		return fmt.Errorf("internal gateway class: %w", err)
	}
	return nil
}

func (c *Client) applyGatewayClass(ctx context.Context, name, controllerName string) error {
	gc := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"morsel.io/managed": "true"},
		},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: gatewayv1.GatewayController(controllerName),
		},
	}
	_, err := c.gw.GatewayV1().GatewayClasses().Get(ctx, name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.gw.GatewayV1().GatewayClasses().Create(ctx, gc, metav1.CreateOptions{})
		return err
	}
	return err
}

// EnsureGateways provisions the external and internal Gateway resources in ns.
// tlsSecretName is the K8s Secret (in ns) containing the wildcard TLS certificate.
// Idempotent — safe to call on every bootstrap run.
func (c *Client) EnsureGateways(ctx context.Context, ns, tlsSecretName string) error {
	if err := c.applyGateway(ctx, ns, GatewayExternal, GatewayClassExternal, tlsSecretName); err != nil {
		return fmt.Errorf("external gateway: %w", err)
	}
	if err := c.applyGateway(ctx, ns, GatewayInternal, GatewayClassInternal, tlsSecretName); err != nil {
		return fmt.Errorf("internal gateway: %w", err)
	}
	return nil
}

func (c *Client) applyGateway(ctx context.Context, ns, name, className, tlsSecretName string) error {
	secretNS := gatewayv1.Namespace(ns)
	fromAll := gatewayv1.NamespacesFromAll
	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{"morsel.io/managed": "true"},
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(className),
			Listeners: []gatewayv1.Listener{
				{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
					TLS: &gatewayv1.GatewayTLSConfig{
						Mode: ptrTo(gatewayv1.TLSModeTerminate),
						CertificateRefs: []gatewayv1.SecretObjectReference{
							{
								Name:      gatewayv1.ObjectName(tlsSecretName),
								Namespace: &secretNS,
							},
						},
					},
					AllowedRoutes: &gatewayv1.AllowedRoutes{
						Namespaces: &gatewayv1.RouteNamespaces{
							From: &fromAll,
						},
					},
				},
			},
		},
	}
	existing, err := c.gw.GatewayV1().Gateways(ns).Get(ctx, name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.gw.GatewayV1().Gateways(ns).Create(ctx, gw, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec = gw.Spec
	_, err = c.gw.GatewayV1().Gateways(ns).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// ApplyAppService creates or updates the Kubernetes Service that exposes an HTTP app.
func (c *Client) ApplyAppService(ctx context.Context, namespace, appName string, port int32) error {
	labels := map[string]string{"morsel.io/app": appName}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appServiceName,
			Namespace: namespace,
			Labels:    map[string]string{"morsel.io/managed": "true", "morsel.io/app": appName},
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: port,
				},
			},
		},
	}
	existing, err := c.cs.CoreV1().Services(namespace).Get(ctx, appServiceName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.CoreV1().Services(namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec.Ports = desired.Spec.Ports
	existing.Spec.Selector = desired.Spec.Selector
	_, err = c.cs.CoreV1().Services(namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// ApplyHTTPRoute creates or updates an HTTPRoute that routes host to the app Service.
// gatewayNS/gatewayName identify the parent Gateway; the backend Service is always
// in the same namespace as the route.
func (c *Client) ApplyHTTPRoute(ctx context.Context, namespace, host, gatewayNS, gatewayName string, port int32) error {
	gwNS := gatewayv1.Namespace(gatewayNS)
	appNS := gatewayv1.Namespace(namespace)
	hostName := gatewayv1.Hostname(host)
	gwPort := gatewayv1.PortNumber(port)
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      httprouteName,
			Namespace: namespace,
			Labels:    map[string]string{"morsel.io/managed": "true"},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:      gatewayv1.ObjectName(gatewayName),
						Namespace: &gwNS,
					},
				},
			},
			Hostnames: []gatewayv1.Hostname{hostName},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name:      gatewayv1.ObjectName(appServiceName),
									Namespace: &appNS,
									Port:      &gwPort,
								},
							},
						},
					},
				},
			},
		},
	}
	existing, err := c.gw.GatewayV1().HTTPRoutes(namespace).Get(ctx, httprouteName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.gw.GatewayV1().HTTPRoutes(namespace).Create(ctx, route, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec = route.Spec
	_, err = c.gw.GatewayV1().HTTPRoutes(namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// DeleteHTTPRoute removes the HTTPRoute from an app namespace.
// Returns nil if the route does not exist.
func (c *Client) DeleteHTTPRoute(ctx context.Context, namespace string) error {
	err := c.gw.GatewayV1().HTTPRoutes(namespace).Delete(ctx, httprouteName, metav1.DeleteOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return err
}

// DeleteAppService removes the Kubernetes Service from an app namespace.
// Returns nil if the service does not exist.
func (c *Client) DeleteAppService(ctx context.Context, namespace string) error {
	err := c.cs.CoreV1().Services(namespace).Delete(ctx, appServiceName, metav1.DeleteOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return err
}

// EnsureAPIHTTPRoute creates or updates an HTTPRoute in ns that routes hostname
// to the morsel-api Service. The route is attached to the morsel-external Gateway.
func (c *Client) EnsureAPIHTTPRoute(ctx context.Context, ns, hostname string) error {
	gwNS := gatewayv1.Namespace(ns)
	port := gatewayv1.PortNumber(apiPort)
	hostName := gatewayv1.Hostname(hostname)
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiName,
			Namespace: ns,
			Labels:    map[string]string{"morsel.io/managed": "true"},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:      gatewayv1.ObjectName(GatewayExternal),
						Namespace: &gwNS,
					},
				},
			},
			Hostnames: []gatewayv1.Hostname{hostName},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName(apiName),
									Port: &port,
								},
							},
						},
					},
				},
			},
		},
	}
	existing, err := c.gw.GatewayV1().HTTPRoutes(ns).Get(ctx, apiName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.gw.GatewayV1().HTTPRoutes(ns).Create(ctx, route, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec = route.Spec
	_, err = c.gw.GatewayV1().HTTPRoutes(ns).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// StoreTLSSecret writes a *tls.Certificate as a kubernetes.io/tls Secret.
// Idempotent — creates or updates.
func (c *Client) StoreTLSSecret(ctx context.Context, namespace, name string, cert *tls.Certificate) error {
	certPEM, keyPEM, err := encodeCertPEM(cert)
	if err != nil {
		return fmt.Errorf("encode cert: %w", err)
	}
	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"morsel.io/managed": "true"},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       certPEM,
			corev1.TLSPrivateKeyKey: keyPEM,
		},
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
	existing.Type = desired.Type
	_, err = c.cs.CoreV1().Secrets(namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// GetTLSCertExpiry reads a kubernetes.io/tls Secret and returns the certificate's NotAfter.
// Returns (nil, nil) when the secret does not exist.
func (c *Client) GetTLSCertExpiry(ctx context.Context, namespace, name string) (*time.Time, error) {
	secret, err := c.cs.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tls secret: %w", err)
	}
	certPEM := secret.Data[corev1.TLSCertKey]
	if len(certPEM) == 0 {
		return nil, nil
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("decode tls cert pem")
	}
	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse tls cert: %w", err)
	}
	expiry := x509Cert.NotAfter
	return &expiry, nil
}

// DeleteAppNamespace removes the Kubernetes namespace for an app and all its resources.
// Returns nil if the namespace does not exist.
func (c *Client) DeleteAppNamespace(ctx context.Context, namespace string) error {
	err := c.cs.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return err
}

// encodeCertPEM encodes a *tls.Certificate into PEM-encoded cert and key bytes.
func encodeCertPEM(cert *tls.Certificate) (certPEM, keyPEM []byte, err error) {
	for _, c := range cert.Certificate {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c})...)
	}
	keyPEM, err = marshalPrivateKey(cert.PrivateKey)
	return certPEM, keyPEM, err
}

func ptrTo[T any](v T) *T { return &v }

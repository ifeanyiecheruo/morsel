package kube

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	wakeProxySecretName = "wake-proxy-token"
	wakeProxyTokenKey   = "token"
	wakeProxyNetpolName = "morsel-wake-proxy-netpol"
)

// EnsureWakeProxy provisions the wake-on-request proxy in the morsel-services
// namespace. It creates a shared token Secret in both morsel-services and apiNS
// so the wake proxy can authenticate to the control plane's /internal/wake
// endpoint. Idempotent — safe to call on every bootstrap run.
func (c *Client) EnsureWakeProxy(ctx context.Context, apiNS, image string) error {
	if err := c.ensureServicesNamespace(ctx); err != nil {
		return fmt.Errorf("namespace: %w", err)
	}
	if err := c.ensureWakeProxyToken(ctx, apiNS); err != nil {
		return fmt.Errorf("token secret: %w", err)
	}
	if err := c.applyWakeProxyNetworkPolicy(ctx); err != nil {
		return fmt.Errorf("network policy: %w", err)
	}
	ctrlPlaneAddr := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", apiName, apiNS, apiPort)
	if err := c.applyWakeProxyDeployment(ctx, image, ctrlPlaneAddr); err != nil {
		return fmt.Errorf("deployment: %w", err)
	}
	if err := c.applyWakeProxyService(ctx); err != nil {
		return fmt.Errorf("service: %w", err)
	}
	return nil
}

func (c *Client) ensureServicesNamespace(ctx context.Context) error {
	labels := map[string]string{
		"morsel.io/managed":  "true",
		"morsel.io/services": "true",
	}
	_, err := c.cs.CoreV1().Namespaces().Get(ctx, wakeProxyNamespace, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: wakeProxyNamespace, Labels: labels},
		}
		_, err = c.cs.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	}
	return err
}

// ensureWakeProxyToken generates a random bearer token on first run and stores
// it as a Secret in both wakeProxyNamespace (for the wake proxy pod) and apiNS
// (for the control plane pod). On subsequent runs it is a no-op.
func (c *Client) ensureWakeProxyToken(ctx context.Context, apiNS string) error {
	existing, err := c.cs.CoreV1().Secrets(wakeProxyNamespace).Get(ctx, wakeProxySecretName, metav1.GetOptions{})
	var token string
	if k8serrors.IsNotFound(err) {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return fmt.Errorf("generate token: %w", err)
		}

		// TODO: Use a Kubernetes Secret of type "kubernetes.io/service-account-token" instead of a random string, which would allow automatic rotation and expiration.
		token = hex.EncodeToString(raw)
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wakeProxySecretName,
				Namespace: wakeProxyNamespace,
				Labels:    map[string]string{"morsel.io/managed": "true"},
			},
			StringData: map[string]string{wakeProxyTokenKey: token},
		}
		if _, err := c.cs.CoreV1().Secrets(wakeProxyNamespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create token secret in %s: %w", wakeProxyNamespace, err)
		}
	} else if err != nil {
		return err
	} else {
		token = string(existing.Data[wakeProxyTokenKey])
	}

	// Mirror the token into the control-plane namespace so the API pod can read
	// it from its own namespace without cross-namespace RBAC.
	_, err = c.cs.CoreV1().Secrets(apiNS).Get(ctx, wakeProxySecretName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		mirror := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wakeProxySecretName,
				Namespace: apiNS,
				Labels:    map[string]string{"morsel.io/managed": "true"},
			},
			StringData: map[string]string{wakeProxyTokenKey: token},
		}
		_, err = c.cs.CoreV1().Secrets(apiNS).Create(ctx, mirror, metav1.CreateOptions{})
		return err
	}
	return err
}

func (c *Client) applyWakeProxyNetworkPolicy(ctx context.Context) error {
	protocolTCP := corev1.ProtocolTCP
	port := intstr.FromInt32(wakeProxyPort)
	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wakeProxyNetpolName,
			Namespace: wakeProxyNamespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"morsel.io/component": wakeProxyService},
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// Envoy proxy pods (spun up by Envoy Gateway) forward HTTPRoute
					// traffic to the wake proxy when an app is hibernated.
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": envoyGatewayNamespace,
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &protocolTCP, Port: &port},
					},
				},
			},
		},
	}
	existing, err := c.cs.NetworkingV1().NetworkPolicies(wakeProxyNamespace).Get(ctx, wakeProxyNetpolName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.NetworkingV1().NetworkPolicies(wakeProxyNamespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	_, err = c.cs.NetworkingV1().NetworkPolicies(wakeProxyNamespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyWakeProxyDeployment(ctx context.Context, image, ctrlPlaneAddr string) error {
	replicas := int32(1)
	labels := map[string]string{"morsel.io/component": wakeProxyService}
	optionalTrue := true
	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wakeProxyService,
			Namespace: wakeProxyNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            wakeProxyService,
							Image:           image,
							ImagePullPolicy: corev1.PullNever,
							Args: []string{
								"svc", "wake-proxy",
								"--addr", fmt.Sprintf(":%d", wakeProxyPort),
								"--api", ctrlPlaneAddr,
							},
							Env: []corev1.EnvVar{
								{
									Name: "WAKE_PROXY_TOKEN",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: wakeProxySecretName,
											},
											Key:      wakeProxyTokenKey,
											Optional: &optionalTrue,
										},
									},
								},
							},
							Ports: []corev1.ContainerPort{
								{ContainerPort: wakeProxyPort, Name: "http"},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt32(wakeProxyPort),
									},
								},
								InitialDelaySeconds: 3,
								PeriodSeconds:       5,
								FailureThreshold:    6,
							},
						},
					},
				},
			},
		},
	}
	existing, err := c.cs.AppsV1().Deployments(wakeProxyNamespace).Get(ctx, wakeProxyService, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.AppsV1().Deployments(wakeProxyNamespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	_, err = c.cs.AppsV1().Deployments(wakeProxyNamespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyWakeProxyService(ctx context.Context) error {
	labels := map[string]string{"morsel.io/component": wakeProxyService}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wakeProxyService,
			Namespace: wakeProxyNamespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       wakeProxyPort,
					TargetPort: intstr.FromInt32(wakeProxyPort),
				},
			},
		},
	}
	existing, err := c.cs.CoreV1().Services(wakeProxyNamespace).Get(ctx, wakeProxyService, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.CoreV1().Services(wakeProxyNamespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec.Ports = desired.Spec.Ports
	existing.Spec.Selector = desired.Spec.Selector
	_, err = c.cs.CoreV1().Services(wakeProxyNamespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

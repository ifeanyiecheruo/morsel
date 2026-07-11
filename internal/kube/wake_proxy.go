package kube

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	wakeProxyNetpolName         = "morsel-wake-proxy-netpol"
	wakeProxyServiceAccountName = "morsel-wake-proxy"
	wakeProxyTokenVolumeName    = "wake-proxy-token"
	wakeProxyTokenAudience      = "morsel-internal-wake"
	wakeProxyTokenMountDir      = "/var/run/secrets/morsel.io/wake-proxy"

	// WakeProxyTokenPath is the path of the projected service account token inside
	// the wake proxy pod. The morsel-ctrl-plane binary reads this file on each
	// request and forwards it as a Bearer token to /internal/wake.
	WakeProxyTokenPath = wakeProxyTokenMountDir + "/token"
)

// EnsureWakeProxy provisions the wake-on-request proxy in the morsel-services
// namespace. The proxy authenticates to the control plane's /internal/wake
// endpoint using a projected service account token (audience: morsel-internal-wake)
// that kubelet rotates automatically. Idempotent — safe to call on every bootstrap run.
func (c *Client) EnsureWakeProxy(ctx context.Context, apiNS, image string) error {
	if err := c.ensureServicesNamespace(ctx); err != nil {
		return fmt.Errorf("namespace: %w", err)
	}
	if err := c.applyWakeProxySA(ctx); err != nil {
		return fmt.Errorf("service account: %w", err)
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

func (c *Client) applyWakeProxySA(ctx context.Context) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wakeProxyServiceAccountName,
			Namespace: wakeProxyNamespace,
			Labels:    map[string]string{"morsel.io/managed": "true"},
		},
	}
	_, err := c.cs.CoreV1().ServiceAccounts(wakeProxyNamespace).Get(ctx, wakeProxyServiceAccountName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.CoreV1().ServiceAccounts(wakeProxyNamespace).Create(ctx, sa, metav1.CreateOptions{})
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
	tokenExpiry := int64(3600)
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
					ServiceAccountName: wakeProxyServiceAccountName,
					Containers: []corev1.Container{
						{
							Name:            wakeProxyService,
							Image:           image,
							ImagePullPolicy: corev1.PullNever,
							Args: []string{
								"run", "wake-proxy",
								"--addr", fmt.Sprintf(":%d", wakeProxyPort),
								"--api", ctrlPlaneAddr,
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
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      wakeProxyTokenVolumeName,
									MountPath: wakeProxyTokenMountDir,
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: wakeProxyTokenVolumeName,
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
										{
											ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
												Audience:          wakeProxyTokenAudience,
												ExpirationSeconds: &tokenExpiry,
												Path:              "token",
											},
										},
									},
								},
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

// VerifyWakeToken validates a projected service account token sent by the wake
// proxy pod to /internal/wake. It calls the Kubernetes TokenReview API to
// confirm the token is authentic, unexpired, and bound to the wake proxy
// service account — without sharing any static secret.
func (c *Client) VerifyWakeToken(ctx context.Context, token string) error {
	tr := &authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token:     token,
			Audiences: []string{wakeProxyTokenAudience},
		},
	}
	result, err := c.cs.AuthenticationV1().TokenReviews().Create(ctx, tr, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("token review: %w", err)
	}
	if !result.Status.Authenticated {
		return fmt.Errorf("token not authenticated")
	}
	want := fmt.Sprintf("system:serviceaccount:%s:%s", wakeProxyNamespace, wakeProxyServiceAccountName)
	if result.Status.User.Username != want {
		return fmt.Errorf("unexpected service account: %s", result.Status.User.Username)
	}
	return nil
}

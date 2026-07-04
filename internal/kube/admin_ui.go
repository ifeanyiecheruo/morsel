package kube

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	adminUIName = "morsel-admin-ui"
	adminUIPort = int32(8090)
)

// EnsureAdminUI provisions the morsel-admin-ui Deployment and Service in ns.
// It contacts the morsel-api via the cluster-local DNS address. Idempotent —
// safe to call on every bootstrap run.
func (c *Client) EnsureAdminUI(ctx context.Context, ns, image string) error {
	apiURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", apiName, ns, apiPort)
	if err := c.applyAdminUIDeployment(ctx, ns, image, apiURL); err != nil {
		return fmt.Errorf("deployment: %w", err)
	}
	if err := c.applyAdminUIService(ctx, ns); err != nil {
		return fmt.Errorf("service: %w", err)
	}
	return nil
}

func (c *Client) applyAdminUIDeployment(ctx context.Context, ns, image, apiURL string) error {
	replicas := int32(1)
	labels := map[string]string{"morsel.io/component": adminUIName}
	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      adminUIName,
			Namespace: ns,
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
							Name:            "admin-ui",
							Image:           image,
							ImagePullPolicy: corev1.PullNever,
							Args: []string{
								"run", "admin-ui",
								"--addr", fmt.Sprintf(":%d", adminUIPort),
								"--api-url", apiURL,
							},
							Ports: []corev1.ContainerPort{
								{ContainerPort: adminUIPort, Name: "http"},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt32(adminUIPort),
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
	existing, err := c.cs.AppsV1().Deployments(ns).Get(ctx, adminUIName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.AppsV1().Deployments(ns).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	_, err = c.cs.AppsV1().Deployments(ns).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyAdminUIService(ctx context.Context, ns string) error {
	labels := map[string]string{"morsel.io/component": adminUIName}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      adminUIName,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       adminUIPort,
					TargetPort: intstr.FromInt32(adminUIPort),
				},
			},
		},
	}
	existing, err := c.cs.CoreV1().Services(ns).Get(ctx, adminUIName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.CoreV1().Services(ns).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec.Ports = desired.Spec.Ports
	existing.Spec.Selector = desired.Spec.Selector
	_, err = c.cs.CoreV1().Services(ns).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

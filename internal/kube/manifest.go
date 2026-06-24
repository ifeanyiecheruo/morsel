package kube

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// morselNamespace is where the Morsel API and Envoy Gateway run; NetworkPolicy
	// allows ingress from here to all app pods.
	morselNamespace = "morsel"

	containerName      = "app"
	serviceAccountName = "morsel-app"
	deploymentName     = "app"
	cronJobName        = "app"
	quotaName          = "morsel-quota"
	limitRangeName     = "morsel-limits"
	networkPolicyName  = "morsel-netpol"
)

// smallTierQuota is hardcoded until F14 introduces dynamic tier configuration.
var smallTierQuota = corev1.ResourceList{
	corev1.ResourcePods:           resource.MustParse("10"),
	corev1.ResourceRequestsCPU:    resource.MustParse("500m"),
	corev1.ResourceRequestsMemory: resource.MustParse("512Mi"),
	corev1.ResourceLimitsCPU:      resource.MustParse("2"),
	corev1.ResourceLimitsMemory:   resource.MustParse("2Gi"),
}

// AppManifest describes the Kubernetes resources to apply for a single app.
type AppManifest struct {
	Namespace string
	AppName   string // used in pod labels
	Type      string // "http", "worker", or "cron"
	Image     string
	Env       map[string]string
	Schedule  string // cron expression; only used when Type is "cron"
}

// Apply is idempotent; safe to call on every deploy.
func (c *Client) Apply(ctx context.Context, m AppManifest) error {
	if err := c.ensureNamespace(ctx, m.Namespace); err != nil {
		return fmt.Errorf("namespace: %w", err)
	}
	if err := c.applyResourceQuota(ctx, m.Namespace); err != nil {
		return fmt.Errorf("resource quota: %w", err)
	}
	if err := c.applyLimitRange(ctx, m.Namespace); err != nil {
		return fmt.Errorf("limit range: %w", err)
	}
	if err := c.applyNetworkPolicy(ctx, m.Namespace); err != nil {
		return fmt.Errorf("network policy: %w", err)
	}
	if err := c.applyServiceAccount(ctx, m.Namespace); err != nil {
		return fmt.Errorf("service account: %w", err)
	}
	switch m.Type {
	case "cron":
		if err := c.applyCronJob(ctx, m); err != nil {
			return fmt.Errorf("cronjob: %w", err)
		}
	default: // "http", "worker"
		if err := c.applyDeployment(ctx, m); err != nil {
			return fmt.Errorf("deployment: %w", err)
		}
	}
	return nil
}

func (c *Client) ensureNamespace(ctx context.Context, name string) error {
	labels := map[string]string{
		"morsel.io/managed": "true",
		"morsel.io/app-ns":  "true",
	}
	_, err := c.cs.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		}
		_, err = c.cs.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		return err
	}
	return err
}

func (c *Client) applyResourceQuota(ctx context.Context, namespace string) error {
	desired := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: quotaName, Namespace: namespace},
		Spec:       corev1.ResourceQuotaSpec{Hard: smallTierQuota},
	}
	existing, err := c.cs.CoreV1().ResourceQuotas(namespace).Get(ctx, quotaName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.CoreV1().ResourceQuotas(namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	_, err = c.cs.CoreV1().ResourceQuotas(namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyLimitRange(ctx context.Context, namespace string) error {
	desired := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: limitRangeName, Namespace: namespace},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Default: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					DefaultRequest: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
					Max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
			},
		},
	}
	existing, err := c.cs.CoreV1().LimitRanges(namespace).Get(ctx, limitRangeName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.CoreV1().LimitRanges(namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	_, err = c.cs.CoreV1().LimitRanges(namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// applyNetworkPolicy allows ingress from same-namespace pods and the morsel
// gateway; all other ingress is denied, blocking cross-sidecar access.
func (c *Client) applyNetworkPolicy(ctx context.Context, namespace string) error {
	protocolTCP := corev1.ProtocolTCP
	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: networkPolicyName, Namespace: namespace},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// Allow ingress from pods in the same namespace.
					From: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{}},
					},
				},
				{
					// Allow ingress from the morsel control-plane namespace on port 8080.
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": morselNamespace,
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &protocolTCP, Port: &intstr.IntOrString{IntVal: 8080}},
					},
				},
			},
		},
	}
	existing, err := c.cs.NetworkingV1().NetworkPolicies(namespace).Get(ctx, networkPolicyName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.NetworkingV1().NetworkPolicies(namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	_, err = c.cs.NetworkingV1().NetworkPolicies(namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyServiceAccount(ctx context.Context, namespace string) error {
	desired := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: namespace,
			Labels:    map[string]string{"morsel.io/managed": "true"},
		},
	}
	_, err := c.cs.CoreV1().ServiceAccounts(namespace).Get(ctx, serviceAccountName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.CoreV1().ServiceAccounts(namespace).Create(ctx, desired, metav1.CreateOptions{})
	}
	return err
}

func (c *Client) applyDeployment(ctx context.Context, m AppManifest) error {
	replicas := int32(1)
	labels := map[string]string{"morsel.io/app": m.AppName}
	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: m.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName,
					Containers: []corev1.Container{
						{
							Name:  containerName,
							Image: m.Image,
							Env:   envVars(m.Env),
						},
					},
				},
			},
		},
	}
	existing, err := c.cs.AppsV1().Deployments(m.Namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.AppsV1().Deployments(m.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	_, err = c.cs.AppsV1().Deployments(m.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyCronJob(ctx context.Context, m AppManifest) error {
	schedule := m.Schedule
	if schedule == "" {
		schedule = "0 * * * *"
	}
	labels := map[string]string{"morsel.io/app": m.AppName}
	desired := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cronJobName,
			Namespace: m.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: schedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec: corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							RestartPolicy:      corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name:  containerName,
									Image: m.Image,
									Env:   envVars(m.Env),
								},
							},
						},
					},
				},
			},
		},
	}
	existing, err := c.cs.BatchV1().CronJobs(m.Namespace).Get(ctx, cronJobName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.BatchV1().CronJobs(m.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	_, err = c.cs.BatchV1().CronJobs(m.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func envVars(env map[string]string) []corev1.EnvVar {
	if len(env) == 0 {
		return nil
	}
	out := make([]corev1.EnvVar, 0, len(env))
	for k, v := range env {
		out = append(out, corev1.EnvVar{Name: k, Value: v})
	}
	return out
}

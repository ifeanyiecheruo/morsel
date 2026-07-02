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
	// morselNamespace is where the Morsel API runs; NetworkPolicy allows ingress
	// from here to all app pods.
	morselNamespace = "morsel"

	containerName      = "app"
	serviceAccountName = "morsel-app"
	deploymentName     = "app"
	cronJobName        = "app"
	quotaName          = "morsel-quota"
	limitRangeName     = "morsel-limits"
	networkPolicyName  = "morsel-netpol"

	registryName         = "registry"
	registryPort         = int32(5000)
	registryImage        = "registry:2"
	registryPortName     = "registry"
	registrySvcName      = "registry"
	registryKubeNodePort = int32(30050) // must match container.registryNodePort
)

// TierLimits holds the resource quota and limit range values for a quota tier.
// Zero values fall back to the small-tier baseline.
type TierLimits struct {
	CPUMilli int // milliCPU limit per container (e.g. 500 = 0.5 cores)
	MemoryMB int // memory limit per container in MB
}

// smallBaseline is used when TierLimits fields are zero.
var smallBaseline = TierLimits{CPUMilli: 500, MemoryMB: 512}

func (t TierLimits) resolved() TierLimits {
	if t.CPUMilli == 0 {
		t.CPUMilli = smallBaseline.CPUMilli
	}
	if t.MemoryMB == 0 {
		t.MemoryMB = smallBaseline.MemoryMB
	}
	return t
}

// AppManifest describes the Kubernetes resources to apply for a single app.
type AppManifest struct {
	Namespace  string
	AppName    string // used in pod labels and the public hostname
	RepoName   string // used in the public hostname: <app>.<repo>.app.<basedomain>
	Type       string // "http", "worker", or "cron"
	Image      string
	Port       int32 // container port; 0 means use appServicePort default
	Env        map[string]string
	Schedule   string     // cron expression; only used when Type is "cron"
	Private    bool       // if true, route through internal gateway class
	BaseDomain string     // e.g. "morsel.localhost"; empty disables routing provisioning
	GatewayNS  string     // namespace where the Gateway resources live (e.g. "morsel")
	Limits     TierLimits // zero value falls back to small-tier baseline
}

// Apply is idempotent; safe to call on every deploy.
// ApplyNamespaceTier updates only the ResourceQuota and LimitRange for a
// namespace when a tier's limits change. Idempotent.
func (c *Client) ApplyNamespaceTier(ctx context.Context, namespace string, limits TierLimits) error {
	if err := c.applyResourceQuota(ctx, namespace, limits); err != nil {
		return fmt.Errorf("resource quota: %w", err)
	}
	if err := c.applyLimitRange(ctx, namespace, limits); err != nil {
		return fmt.Errorf("limit range: %w", err)
	}
	return nil
}

func (c *Client) Apply(ctx context.Context, m AppManifest) error {
	if err := c.ensureNamespace(ctx, m.Namespace); err != nil {
		return fmt.Errorf("namespace: %w", err)
	}
	if err := c.applyResourceQuota(ctx, m.Namespace, m.Limits); err != nil {
		return fmt.Errorf("resource quota: %w", err)
	}
	if err := c.applyLimitRange(ctx, m.Namespace, m.Limits); err != nil {
		return fmt.Errorf("limit range: %w", err)
	}
	if err := c.applyNetworkPolicy(ctx, m.Namespace, resolvePort(m)); err != nil {
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
	if m.Type == "http" && m.BaseDomain != "" {
		port := resolvePort(m)
		if err := c.ApplyAppService(ctx, m.Namespace, m.AppName, port); err != nil {
			return fmt.Errorf("app service: %w", err)
		}
		host := appHostname(m.AppName, m.RepoName, m.BaseDomain)
		gatewayNS := m.GatewayNS
		if gatewayNS == "" {
			gatewayNS = morselNamespace
		}
		gatewayName := GatewayExternal
		if m.Private {
			gatewayName = GatewayInternal
		}
		if err := c.ApplyHTTPRoute(ctx, m.Namespace, host, gatewayNS, gatewayName, port); err != nil {
			return fmt.Errorf("httproute: %w", err)
		}
	}
	return nil
}

// Delete removes all Kubernetes resources for an app: routing resources, Service,
// and the namespace itself (which cascades to Deployment/CronJob and other resources).
func (c *Client) Delete(ctx context.Context, namespace string) error {
	if err := c.DeleteHTTPRoute(ctx, namespace); err != nil {
		return fmt.Errorf("delete httproute: %w", err)
	}
	if err := c.DeleteAppService(ctx, namespace); err != nil {
		return fmt.Errorf("delete app service: %w", err)
	}
	if err := c.DeleteAppNamespace(ctx, namespace); err != nil {
		return fmt.Errorf("delete namespace: %w", err)
	}
	return nil
}

// appHostname derives the public hostname for an app.
func appHostname(appName, repoName, baseDomain string) string {
	return appName + "." + repoName + ".app." + baseDomain
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

func (c *Client) applyResourceQuota(ctx context.Context, namespace string, limits TierLimits) error {
	lim := limits.resolved()
	hard := corev1.ResourceList{
		corev1.ResourcePods:           resource.MustParse("10"),
		corev1.ResourceRequestsCPU:    resource.MustParse(fmt.Sprintf("%dm", lim.CPUMilli/4)),
		corev1.ResourceRequestsMemory: resource.MustParse(fmt.Sprintf("%dMi", lim.MemoryMB/4)),
		corev1.ResourceLimitsCPU:      resource.MustParse(fmt.Sprintf("%dm", lim.CPUMilli)),
		corev1.ResourceLimitsMemory:   resource.MustParse(fmt.Sprintf("%dMi", lim.MemoryMB)),
	}
	desired := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: quotaName, Namespace: namespace},
		Spec:       corev1.ResourceQuotaSpec{Hard: hard},
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

func (c *Client) applyLimitRange(ctx context.Context, namespace string, limits TierLimits) error {
	lim := limits.resolved()
	desired := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: limitRangeName, Namespace: namespace},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Default: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%dm", lim.CPUMilli)),
						corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", lim.MemoryMB)),
					},
					DefaultRequest: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
					Max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%dm", lim.CPUMilli)),
						corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", lim.MemoryMB)),
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
func (c *Client) applyNetworkPolicy(ctx context.Context, namespace string, port int32) error {
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
					// Allow ingress from the morsel namespace (control-plane) and
					// the envoy-gateway-system namespace (Envoy proxy pods) on the
					// app port. The Envoy proxy is spun up by Envoy Gateway in its
					// own namespace, not in morsel.
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": morselNamespace,
								},
							},
						},
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": envoyGatewayNamespace,
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &protocolTCP, Port: &intstr.IntOrString{IntVal: port}},
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
							Env:   envVars(m.Env, resolvePort(m)),
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
									Env:   envVars(m.Env, 0),
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

func envVars(env map[string]string, port int32) []corev1.EnvVar {
	out := make([]corev1.EnvVar, 0, len(env)+1)
	if port > 0 {
		out = append(out, corev1.EnvVar{Name: "PORT", Value: fmt.Sprintf("%d", port)})
	}
	for k, v := range env {
		out = append(out, corev1.EnvVar{Name: k, Value: v})
	}
	return out
}

// EnsureRegistry provisions the registry:2 Deployment and LoadBalancer Service
// in the given namespace. Idempotent — safe to call on every bootstrap run.
// The registry is exposed as a LoadBalancer on port 5000. Docker Desktop
// surfaces LoadBalancer services at localhost, making localhost:5000 reachable
// from both the host (for docker push) and from pods (for image pull).
func (c *Client) EnsureRegistry(ctx context.Context, namespace string) error {
	if err := c.ensureNamespace(ctx, namespace); err != nil {
		return fmt.Errorf("namespace: %w", err)
	}
	if err := c.applyRegistryDeployment(ctx, namespace); err != nil {
		return fmt.Errorf("registry deployment: %w", err)
	}
	if err := c.applyRegistryService(ctx, namespace); err != nil {
		return fmt.Errorf("registry service: %w", err)
	}
	return nil
}

func (c *Client) applyRegistryDeployment(ctx context.Context, namespace string) error {
	replicas := int32(1)
	labels := map[string]string{"morsel.io/component": registryName}
	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      registryName,
			Namespace: namespace,
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
							Name:  registryName,
							Image: registryImage,
							Ports: []corev1.ContainerPort{
								{ContainerPort: registryPort, Name: registryPortName},
							},
						},
					},
				},
			},
		},
	}
	existing, err := c.cs.AppsV1().Deployments(namespace).Get(ctx, registryName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.AppsV1().Deployments(namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	_, err = c.cs.AppsV1().Deployments(namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyRegistryService(ctx context.Context, namespace string) error {
	port := corev1.ServicePort{
		Name:       registryPortName,
		Port:       registryPort,
		TargetPort: intstr.FromInt32(registryPort),
		NodePort:   registryKubeNodePort,
	}
	labels := map[string]string{"morsel.io/component": registryName}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      registrySvcName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: map[string]string{"morsel.io/component": registryName},
			Ports:    []corev1.ServicePort{port},
		},
	}
	existing, err := c.cs.CoreV1().Services(namespace).Get(ctx, registrySvcName, metav1.GetOptions{})
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

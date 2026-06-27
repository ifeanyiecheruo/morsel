package kube

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	apiName               = "morsel-api"
	apiServiceAccountName = "morsel-api"
	apiContainerName      = "api"
	apiPort               = int32(8080)
	apiNodePort           = int32(30080)
	apiDataMount          = "/data"
	apiPVCName            = "morsel-api-data"

	// MorselUID and MorselGID are the uid/gid of the morsel user built into the
	// morsel-api image. They are passed as --build-arg to docker/podman build so
	// the Dockerfile and the pod SecurityContext stay in sync from a single source.
	MorselUID = 1000
	MorselGID = 1000
)

// EnsureAPI provisions the morsel-api PVC, Deployment, and NodePort Service in
// ns. dbPath is the path inside the container where the SQLite database lives
// (e.g. /data/morsel.db); its parent directory is the PVC mount point.
// Idempotent — safe to call on every bootstrap run.
func (c *Client) EnsureAPI(ctx context.Context, ns, image, dbPath string) error {
	if err := c.ensureNamespace(ctx, ns); err != nil {
		return fmt.Errorf("namespace: %w", err)
	}
	if err := c.applyAPIRBAC(ctx, ns); err != nil {
		return fmt.Errorf("api rbac: %w", err)
	}
	if err := c.applyAPIPVC(ctx, ns); err != nil {
		return fmt.Errorf("api data pvc: %w", err)
	}
	if err := c.applyAPIDeployment(ctx, ns, image, dbPath); err != nil {
		return fmt.Errorf("api deployment: %w", err)
	}
	if err := c.applyAPIService(ctx, ns); err != nil {
		return fmt.Errorf("api service: %w", err)
	}
	return nil
}

func (c *Client) applyAPIRBAC(ctx context.Context, ns string) error {
	// Service account the morsel-api pod runs as.
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: apiServiceAccountName, Namespace: ns},
	}
	_, err := c.cs.CoreV1().ServiceAccounts(ns).Get(ctx, apiServiceAccountName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		if _, err = c.cs.CoreV1().ServiceAccounts(ns).Create(ctx, sa, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("service account: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("service account: %w", err)
	}

	// ClusterRole grants the permissions needed to provision app namespaces and
	// resources across the cluster.
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: apiName},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"namespaces", "serviceaccounts", "services",
					"persistentvolumeclaims", "pods", "resourcequotas", "limitranges"},
				Verbs: []string{"get", "list", "create", "update", "patch", "delete"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments", "replicasets"},
				Verbs:     []string{"get", "list", "create", "update", "patch", "delete"},
			},
			{
				APIGroups: []string{"batch"},
				Resources: []string{"cronjobs", "jobs"},
				Verbs:     []string{"get", "list", "create", "update", "patch", "delete"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"networkpolicies"},
				Verbs:     []string{"get", "list", "create", "update", "patch", "delete"},
			},
			{
				APIGroups: []string{"gateway.networking.k8s.io"},
				Resources: []string{"gatewayclasses", "gateways", "httproutes"},
				Verbs:     []string{"get", "list", "create", "update", "patch", "delete"},
			},
		},
	}
	existing, err := c.cs.RbacV1().ClusterRoles().Get(ctx, apiName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		if _, err = c.cs.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("cluster role: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("cluster role: %w", err)
	} else {
		existing.Rules = cr.Rules
		if _, err = c.cs.RbacV1().ClusterRoles().Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("cluster role: %w", err)
		}
	}

	// ClusterRoleBinding wires the service account to the ClusterRole.
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: apiName},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     apiName,
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: apiServiceAccountName, Namespace: ns},
		},
	}
	_, err = c.cs.RbacV1().ClusterRoleBindings().Get(ctx, apiName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		if _, err = c.cs.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("cluster role binding: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("cluster role binding: %w", err)
	}

	// Role grants access to secrets in the control-plane namespace only.
	// Using a namespaced Role (not ClusterRole) limits scope to ns.
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: apiName, Namespace: ns},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "list", "create", "update", "patch", "delete"},
			},
		},
	}
	existingRole, err := c.cs.RbacV1().Roles(ns).Get(ctx, apiName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		if _, err = c.cs.RbacV1().Roles(ns).Create(ctx, role, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("secrets role: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("secrets role: %w", err)
	} else {
		existingRole.Rules = role.Rules
		if _, err = c.cs.RbacV1().Roles(ns).Update(ctx, existingRole, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("secrets role: %w", err)
		}
	}

	// RoleBinding wires the morsel-api service account to the secrets Role.
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: apiName, Namespace: ns},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     apiName,
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: apiServiceAccountName, Namespace: ns},
		},
	}
	_, err = c.cs.RbacV1().RoleBindings(ns).Get(ctx, apiName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		if _, err = c.cs.RbacV1().RoleBindings(ns).Create(ctx, rb, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("secrets role binding: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("secrets role binding: %w", err)
	}
	return nil
}

func (c *Client) applyAPIPVC(ctx context.Context, ns string) error {
	desired := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiPVCName,
			Namespace: ns,
			Labels:    map[string]string{"morsel.io/component": apiName},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
			// StorageClassName omitted — uses the cluster's default StorageClass.
		},
	}
	_, err := c.cs.CoreV1().PersistentVolumeClaims(ns).Get(ctx, apiPVCName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.CoreV1().PersistentVolumeClaims(ns).Create(ctx, desired, metav1.CreateOptions{})
	}
	return err
}

func (c *Client) applyAPIDeployment(ctx context.Context, ns, image, dbPath string) error {
	replicas := int32(1)
	labels := map[string]string{"morsel.io/component": apiName}

	uid := int64(MorselUID)
	gid := int64(MorselGID)
	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiName,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: apiServiceAccountName,
					// FSGroup causes kubelet to chown the PVC mount to gid 1000
					// before the container starts, making it writable by the morsel user.
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:  &uid,
						RunAsGroup: &gid,
						FSGroup:    &gid,
					},
					Containers: []corev1.Container{
						{
							Name:            apiContainerName,
							Image:           image,
							ImagePullPolicy: corev1.PullNever,
							Args: []string{
								"--platform", "local",
								"--db", dbPath,
							},
							Env: []corev1.EnvVar{
								{Name: "MORSEL_LOCAL_DATA_DIR", Value: apiDataMount},
							},
							Ports: []corev1.ContainerPort{
								{ContainerPort: apiPort, Name: "http"},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt32(apiPort),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								FailureThreshold:    12,
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: apiDataMount},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: apiPVCName,
								},
							},
						},
					},
				},
			},
		},
	}
	existing, err := c.cs.AppsV1().Deployments(ns).Get(ctx, apiName, metav1.GetOptions{})
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

// WaitForAPIReady polls until the morsel-api deployment reaches ReadyReplicas ≥ 1
// or timeout elapses. It fails immediately on terminal pod failure states.
func (c *Client) WaitForAPIReady(ctx context.Context, ns string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		deploy, err := c.cs.AppsV1().Deployments(ns).Get(ctx, apiName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get morsel-api deployment: %w", err)
		}
		if deploy.Status.ReadyReplicas >= 1 {
			return nil
		}
		if err := c.checkAPIFailures(ctx, ns); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("morsel-api deployment not ready after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (c *Client) checkAPIFailures(ctx context.Context, ns string) error {
	pods, err := c.cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "morsel.io/component=" + apiName,
	})
	if err != nil {
		return nil
	}
	var reasons []string
	for i := range pods.Items {
		if pods.Items[i].Status.Phase == corev1.PodSucceeded {
			continue
		}
		for _, cs := range pods.Items[i].Status.ContainerStatuses {
			if cs.State.Waiting != nil && terminalPodReasons[cs.State.Waiting.Reason] {
				msg := cs.State.Waiting.Reason
				if cs.State.Waiting.Message != "" {
					msg += ": " + cs.State.Waiting.Message
				}
				reasons = append(reasons, msg)
			}
		}
	}
	if len(reasons) > 0 {
		return fmt.Errorf("morsel-api pod failed: %s", strings.Join(reasons, "; "))
	}
	return nil
}

func (c *Client) applyAPIService(ctx context.Context, ns string) error {
	labels := map[string]string{"morsel.io/component": apiName}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiName,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       apiPort,
					TargetPort: intstr.FromInt32(apiPort),
					NodePort:   apiNodePort,
				},
			},
		},
	}
	existing, err := c.cs.CoreV1().Services(ns).Get(ctx, apiName, metav1.GetOptions{})
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

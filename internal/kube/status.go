package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AppStatus returns "unknown" when the Kubernetes resource cannot be queried.
func (c *Client) AppStatus(ctx context.Context, namespace, appType string) string {
	if appType == "cron" {
		return c.cronJobStatus(ctx, namespace)
	}
	return c.deploymentStatus(ctx, namespace)
}

func (c *Client) deploymentStatus(ctx context.Context, namespace string) string {
	deploy, err := c.cs.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return "unknown"
	}
	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}
	ready := deploy.Status.ReadyReplicas
	if ready >= desired {
		return "running"
	}
	if ready == 0 {
		return "pending"
	}
	return fmt.Sprintf("degraded (%d/%d ready)", ready, desired)
}

func (c *Client) cronJobStatus(ctx context.Context, namespace string) string {
	cronJob, err := c.cs.BatchV1().CronJobs(namespace).Get(ctx, cronJobName, metav1.GetOptions{})
	if err != nil {
		return "unknown"
	}
	if cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend {
		return "suspended"
	}
	if len(cronJob.Status.Active) > 0 {
		return "running"
	}
	if cronJob.Status.LastSuccessfulTime != nil {
		return "scheduled"
	}
	return "pending"
}

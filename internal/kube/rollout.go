package kube

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	rolloutPollInterval = 5 * time.Second
	rolloutTimeout      = 5 * time.Minute
)

// terminalPodReasons are container waiting reasons that will never self-resolve;
// we fail fast instead of waiting for the rollout timeout.
var terminalPodReasons = map[string]bool{
	"ImagePullBackOff":           true,
	"ErrImagePull":               true,
	"InvalidImageName":           true,
	"CrashLoopBackOff":           true,
	"CreateContainerConfigError": true,
	"RunContainerError":          true,
}

// WatchDeploymentRollout polls until the desired replica count is ready or
// rolloutTimeout elapses. It fails immediately if any pod enters a terminal
// failure state (e.g. ImagePullBackOff, CrashLoopBackOff).
func (c *Client) WatchDeploymentRollout(ctx context.Context, namespace string) error {
	deadline := time.Now().Add(rolloutTimeout)
	for {
		deploy, err := c.cs.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get deployment: %w", err)
		}
		desired := int32(1)
		if deploy.Spec.Replicas != nil {
			desired = *deploy.Spec.Replicas
		}
		if deploy.Status.ReadyReplicas >= desired {
			return nil
		}

		if err := c.checkPodFailures(ctx, namespace); err != nil {
			return err
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("deployment not ready after %s (%d/%d replicas ready)",
				rolloutTimeout, deploy.Status.ReadyReplicas, desired)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(rolloutPollInterval):
		}
	}
}

// checkPodFailures returns a non-nil error if any pod in namespace has a
// container in a terminal waiting state that will not self-heal.
func (c *Client) checkPodFailures(ctx context.Context, namespace string) error {
	pods, err := c.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil // treat list failure as transient; let the timeout fire
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
		return fmt.Errorf("pod failed: %s", strings.Join(reasons, "; "))
	}
	return nil
}

func (c *Client) RollbackDeployment(ctx context.Context, namespace, lastHealthyImage string) error {
	deploy, err := c.cs.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get deployment for rollback: %w", err)
	}
	for i := range deploy.Spec.Template.Spec.Containers {
		if deploy.Spec.Template.Spec.Containers[i].Name == containerName {
			deploy.Spec.Template.Spec.Containers[i].Image = lastHealthyImage
		}
	}
	_, err = c.cs.AppsV1().Deployments(namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	return err
}

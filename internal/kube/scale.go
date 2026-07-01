package kube

import (
	"context"
	"fmt"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	// wakeProxyService is the name of the wake-on-request proxy Service in the morsel-services namespace.
	wakeProxyService = "wake-proxy"
	// wakeProxyNamespace is the namespace where the wake proxy runs.
	wakeProxyNamespace = "morsel-services"
	// wakeProxyPort is the port the wake proxy listens on.
	wakeProxyPort = int32(8080)
)

// ScaleDeployment sets the replica count for the app Deployment in namespace.
func (c *Client) ScaleDeployment(ctx context.Context, namespace string, replicas int32) error {
	deploy, err := c.cs.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	deploy.Spec.Replicas = &replicas
	_, err = c.cs.AppsV1().Deployments(namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	return err
}

// SuspendCronJob sets spec.suspend=true on the CronJob in namespace, pausing all future runs.
func (c *Client) SuspendCronJob(ctx context.Context, namespace string) error {
	cj, err := c.cs.BatchV1().CronJobs(namespace).Get(ctx, cronJobName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get cronjob: %w", err)
	}
	suspend := true
	cj.Spec.Suspend = &suspend
	_, err = c.cs.BatchV1().CronJobs(namespace).Update(ctx, cj, metav1.UpdateOptions{})
	return err
}

// UnsuspendCronJob clears spec.suspend on the CronJob in namespace, resuming its schedule.
func (c *Client) UnsuspendCronJob(ctx context.Context, namespace string) error {
	cj, err := c.cs.BatchV1().CronJobs(namespace).Get(ctx, cronJobName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get cronjob: %w", err)
	}
	suspend := false
	cj.Spec.Suspend = &suspend
	_, err = c.cs.BatchV1().CronJobs(namespace).Update(ctx, cj, metav1.UpdateOptions{})
	return err
}

// RouteToWakeProxy updates the HTTPRoute in namespace to forward traffic to the
// shared wake-on-request proxy in morsel-services instead of the app Service.
func (c *Client) RouteToWakeProxy(ctx context.Context, namespace, host, gatewayNS, gatewayName string) error {
	gwNS := gatewayv1.Namespace(gatewayNS)
	proxyNS := gatewayv1.Namespace(wakeProxyNamespace)
	hostName := gatewayv1.Hostname(host)
	proxyPort := gatewayv1.PortNumber(wakeProxyPort)

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
									Name:      gatewayv1.ObjectName(wakeProxyService),
									Namespace: &proxyNS,
									Port:      &proxyPort,
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

// RestoreHTTPRoute updates the HTTPRoute in namespace to forward traffic back to
// the app's own Service after waking from hibernation.
func (c *Client) RestoreHTTPRoute(ctx context.Context, namespace, host, gatewayNS, gatewayName string, port int32) error {
	return c.ApplyHTTPRoute(ctx, namespace, host, gatewayNS, gatewayName, port)
}

// WatchDeploymentReady polls until the Deployment in namespace has ≥1 ready
// replica or timeout elapses.
func (c *Client) WatchDeploymentReady(ctx context.Context, namespace string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		deploy, err := c.cs.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if err == nil && deploy.Status.ReadyReplicas >= 1 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("deployment not ready after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(rolloutPollInterval):
		}
	}
}

// AppReplicaCounts returns desired and ready replica counts for an app.
// Returns (0, 0) for cron apps and when the resource is not found.
func (c *Client) AppReplicaCounts(ctx context.Context, namespace, appType string) (desired, ready int32) {
	if appType == "cron" {
		return 0, 0
	}
	deploy, err := c.cs.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return 0, 0
	}
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}
	ready = deploy.Status.ReadyReplicas
	return desired, ready
}

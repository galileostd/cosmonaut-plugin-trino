// Package client — Kubernetes access for fetching Trino service logs.
//
// The Trino plugin normally talks to Trino over HTTP (the coordinator REST
// API). Service logs are different: they live in the coordinator *pod*, so we
// need a Kubernetes client to read them. This file adds that capability.
package client

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// K8sClient wraps a Kubernetes clientset for pod-log access.
type K8sClient struct {
	clientset *kubernetes.Clientset
}

// NewK8sClient builds a Kubernetes client from the in-cluster config.
func NewK8sClient() (*K8sClient, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("loading in-cluster config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("creating clientset: %w", err)
	}
	return &K8sClient{clientset: clientset}, nil
}

// DefaultTrinoSelector locates the Trino coordinator pod when the component
// config does not override it.
const DefaultTrinoSelector = "app.kubernetes.io/name=trino,app.kubernetes.io/component=coordinator"

// GetServiceLogs returns logs from the Trino coordinator pod.
//
// namespace and selector come from the component config (service_namespace /
// pod_selector); both fall back to sensible defaults when empty. Passing the
// selector through config is what lets one plugin image serve multiple distinct
// Trino deployments.
func (c *K8sClient) GetServiceLogs(ctx context.Context, namespace, selector string, tailLines int64) ([]string, error) {
	if namespace == "" {
		namespace = "default"
	}
	if selector == "" {
		selector = DefaultTrinoSelector
	}

	pods, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
		Limit:         1,
	})
	if err != nil {
		return nil, fmt.Errorf("listing trino pods (selector %q): %w", selector, err)
	}
	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no trino pod found in namespace %q with selector %q", namespace, selector)
	}
	podName := pods.Items[0].Name

	opts := &corev1.PodLogOptions{}
	if tailLines > 0 {
		opts.TailLines = &tailLines
	}

	raw, err := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, opts).DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting logs for trino pod %s/%s: %w", namespace, podName, err)
	}

	return splitLines(raw), nil
}

// splitLines converts a raw log byte buffer into individual lines.
func splitLines(raw []byte) []string {
	var lines []string
	current := ""
	for _, b := range raw {
		if b == '\n' {
			if current != "" {
				lines = append(lines, current)
			}
			current = ""
		} else {
			current += string(b)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
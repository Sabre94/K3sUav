package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/k3suav/uav-monitor/pkg/config"
	"github.com/k3suav/uav-monitor/pkg/models"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client is a Kubernetes client wrapper for UAV CRD operations
type Client struct {
	dynamicClient dynamic.Interface
	config        *config.Config
	gvr           schema.GroupVersionResource
}

// NewClient creates a new Kubernetes client
func NewClient(cfg *config.Config) (*Client, error) {
	var k8sConfig *rest.Config
	var err error

	// Try to use in-cluster config first, then kubeconfig
	if cfg.Kubernetes.KubeconfigPath == "" {
		k8sConfig, err = rest.InClusterConfig()
		if err != nil {
			// Fall back to default kubeconfig location
			kubeconfigPath := clientcmd.RecommendedHomeFile
			k8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
			if err != nil {
				return nil, fmt.Errorf("failed to build kubernetes config: %w", err)
			}
		}
	} else {
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", cfg.Kubernetes.KubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build kubernetes config from %s: %w", cfg.Kubernetes.KubeconfigPath, err)
		}
	}

	// Create dynamic client
	dynamicClient, err := dynamic.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Define GVR (GroupVersionResource)
	gvr := schema.GroupVersionResource{
		Group:    cfg.Kubernetes.CRDGroup,
		Version:  cfg.Kubernetes.CRDVersion,
		Resource: "uavmetrics",
	}

	return &Client{
		dynamicClient: dynamicClient,
		config:        cfg,
		gvr:           gvr,
	}, nil
}

// CreateOrUpdateUAVMetrics creates or updates a UAVMetrics CRD
func (c *Client) CreateOrUpdateUAVMetrics(ctx context.Context, metrics *models.UAVMetrics) error {
	// Convert metrics to unstructured data
	unstructuredData, err := c.metricsToUnstructured(metrics)
	if err != nil {
		return fmt.Errorf("failed to convert metrics to unstructured: %w", err)
	}

	// Set metadata
	name := fmt.Sprintf("uav-%s", metrics.NodeName)
	unstructuredData.SetName(name)
	unstructuredData.SetNamespace(c.config.Kubernetes.Namespace)

	// Add labels
	labels := map[string]string{
		"app":       "uav-agent",
		"node-name": metrics.NodeName,
	}
	unstructuredData.SetLabels(labels)

	// Try to get existing resource
	existing, err := c.dynamicClient.Resource(c.gvr).
		Namespace(c.config.Kubernetes.Namespace).
		Get(ctx, name, metav1.GetOptions{})

	if err != nil {
		// Resource doesn't exist, create it
		_, err = c.dynamicClient.Resource(c.gvr).
			Namespace(c.config.Kubernetes.Namespace).
			Create(ctx, unstructuredData, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create UAVMetrics: %w", err)
		}
		return nil
	}

	// Resource exists, update it
	unstructuredData.SetResourceVersion(existing.GetResourceVersion())
	_, err = c.dynamicClient.Resource(c.gvr).
		Namespace(c.config.Kubernetes.Namespace).
		Update(ctx, unstructuredData, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update UAVMetrics: %w", err)
	}

	return nil
}

// CreateOrUpdateWithRetry creates or updates with retry logic
func (c *Client) CreateOrUpdateWithRetry(ctx context.Context, metrics *models.UAVMetrics) error {
	var lastErr error

	for attempt := 0; attempt <= c.config.Kubernetes.RetryAttempts; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.config.Kubernetes.RetryDelay):
			}
		}

		err := c.CreateOrUpdateUAVMetrics(ctx, metrics)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("failed after %d attempts: %w", c.config.Kubernetes.RetryAttempts+1, lastErr)
}

// GetUAVMetrics retrieves a UAVMetrics CRD
func (c *Client) GetUAVMetrics(ctx context.Context, nodeName string) (*models.UAVMetrics, error) {
	name := fmt.Sprintf("uav-%s", nodeName)

	unstructuredData, err := c.dynamicClient.Resource(c.gvr).
		Namespace(c.config.Kubernetes.Namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get UAVMetrics: %w", err)
	}

	// Convert unstructured to metrics
	metrics, err := c.unstructuredToMetrics(unstructuredData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured to metrics: %w", err)
	}

	return metrics, nil
}

// ListUAVMetrics lists all UAVMetrics CRDs
func (c *Client) ListUAVMetrics(ctx context.Context) ([]*models.UAVMetrics, error) {
	unstructuredList, err := c.dynamicClient.Resource(c.gvr).
		Namespace(c.config.Kubernetes.Namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list UAVMetrics: %w", err)
	}

	metrics := make([]*models.UAVMetrics, 0, len(unstructuredList.Items))
	for _, item := range unstructuredList.Items {
		m, err := c.unstructuredToMetrics(&item)
		if err != nil {
			// Log error but continue with other items
			continue
		}
		metrics = append(metrics, m)
	}

	return metrics, nil
}

// DeleteUAVMetrics deletes a UAVMetrics CRD
func (c *Client) DeleteUAVMetrics(ctx context.Context, nodeName string) error {
	name := fmt.Sprintf("uav-%s", nodeName)

	err := c.dynamicClient.Resource(c.gvr).
		Namespace(c.config.Kubernetes.Namespace).
		Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete UAVMetrics: %w", err)
	}

	return nil
}

// UpdateStatus updates the status subresource
func (c *Client) UpdateStatus(ctx context.Context, nodeName string, phase string) error {
	name := fmt.Sprintf("uav-%s", nodeName)

	// Get current resource
	unstructuredData, err := c.dynamicClient.Resource(c.gvr).
		Namespace(c.config.Kubernetes.Namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get UAVMetrics for status update: %w", err)
	}

	// Update status
	status := map[string]interface{}{
		"phase":       phase,
		"lastUpdated": time.Now().Format(time.RFC3339),
	}

	if err := unstructured.SetNestedMap(unstructuredData.Object, status, "status"); err != nil {
		return fmt.Errorf("failed to set status: %w", err)
	}

	// Update status subresource
	_, err = c.dynamicClient.Resource(c.gvr).
		Namespace(c.config.Kubernetes.Namespace).
		UpdateStatus(ctx, unstructuredData, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

// Helper functions

func (c *Client) metricsToUnstructured(metrics *models.UAVMetrics) (*unstructured.Unstructured, error) {
	// Convert metrics to JSON
	data, err := json.Marshal(metrics)
	if err != nil {
		return nil, err
	}

	// Convert JSON to map
	var spec map[string]interface{}
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}

	// Create unstructured object
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", c.config.Kubernetes.CRDGroup, c.config.Kubernetes.CRDVersion),
			"kind":       "UAVMetrics",
			"spec":       spec,
		},
	}

	return obj, nil
}

func (c *Client) unstructuredToMetrics(obj *unstructured.Unstructured) (*models.UAVMetrics, error) {
	// Extract spec
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return nil, fmt.Errorf("spec not found in unstructured object")
	}

	// Convert spec to JSON
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}

	// Unmarshal to metrics
	var metrics models.UAVMetrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return nil, err
	}

	return &metrics, nil
}

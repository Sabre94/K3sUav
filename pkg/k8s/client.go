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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client K8s客户端
type Client struct {
	dynamicClient dynamic.Interface
	clientset     *kubernetes.Clientset
	config        *config.Config
	gvr           schema.GroupVersionResource
}

// NewClient 创建K8s客户端
func NewClient(cfg *config.Config) (*Client, error) {
	// 使用in-cluster配置或kubeconfig
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		// 回退到默认kubeconfig
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			return nil, fmt.Errorf("failed to build k8s config: %w", err)
		}
	}

	// 创建dynamic client
	dynamicClient, err := dynamic.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// 创建clientset
	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	// 定义GVR (GroupVersionResource)
	gvr := schema.GroupVersionResource{
		Group:    cfg.Kubernetes.CRDGroup,
		Version:  cfg.Kubernetes.CRDVersion,
		Resource: "uavmetrics",
	}

	return &Client{
		dynamicClient: dynamicClient,
		clientset:     clientset,
		config:        cfg,
		gvr:           gvr,
	}, nil
}

// CreateOrUpdateWithRetry 创建或更新UAVMetrics（带重试）
func (c *Client) CreateOrUpdateWithRetry(ctx context.Context, metrics *models.UAVMetrics) error {
	var lastErr error

	for attempt := 0; attempt <= c.config.Kubernetes.RetryAttempts; attempt++ {
		if attempt > 0 {
			// 重试前等待
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.config.Kubernetes.RetryDelay):
			}
		}

		err := c.createOrUpdate(ctx, metrics)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("failed after %d attempts: %w", c.config.Kubernetes.RetryAttempts+1, lastErr)
}

// createOrUpdate 创建或更新UAVMetrics
func (c *Client) createOrUpdate(ctx context.Context, metrics *models.UAVMetrics) error {
	// 转换为unstructured格式
	obj, err := c.toUnstructured(metrics)
	if err != nil {
		return fmt.Errorf("failed to convert to unstructured: %w", err)
	}

	// 设置资源名称和命名空间
	name := fmt.Sprintf("uav-%s", metrics.NodeName)
	obj.SetName(name)
	obj.SetNamespace(c.config.Kubernetes.Namespace)
	obj.SetLabels(map[string]string{
		"app":       "uav-agent",
		"node-name": metrics.NodeName,
	})

	// 尝试获取现有资源
	existing, err := c.dynamicClient.Resource(c.gvr).
		Namespace(c.config.Kubernetes.Namespace).
		Get(ctx, name, metav1.GetOptions{})

	if err != nil {
		// 资源不存在，创建
		_, err = c.dynamicClient.Resource(c.gvr).
			Namespace(c.config.Kubernetes.Namespace).
			Create(ctx, obj, metav1.CreateOptions{})
		return err
	}

	// 资源存在，更新
	obj.SetResourceVersion(existing.GetResourceVersion())
	_, err = c.dynamicClient.Resource(c.gvr).
		Namespace(c.config.Kubernetes.Namespace).
		Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

// UpdateStatus 更新status子资源
func (c *Client) UpdateStatus(ctx context.Context, nodeName string, phase string) error {
	name := fmt.Sprintf("uav-%s", nodeName)

	// 获取当前资源
	obj, err := c.dynamicClient.Resource(c.gvr).
		Namespace(c.config.Kubernetes.Namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get UAVMetrics: %w", err)
	}

	// 更新status
	status := map[string]interface{}{
		"phase":       phase,
		"lastUpdated": time.Now().Format(time.RFC3339),
	}

	if err := unstructured.SetNestedMap(obj.Object, status, "status"); err != nil {
		return fmt.Errorf("failed to set status: %w", err)
	}

	// 更新status子资源
	_, err = c.dynamicClient.Resource(c.gvr).
		Namespace(c.config.Kubernetes.Namespace).
		UpdateStatus(ctx, obj, metav1.UpdateOptions{})
	return err
}

// toUnstructured 将UAVMetrics转换为unstructured格式
func (c *Client) toUnstructured(metrics *models.UAVMetrics) (*unstructured.Unstructured, error) {
	data, err := json.Marshal(metrics)
	if err != nil {
		return nil, err
	}

	var spec map[string]interface{}
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", c.config.Kubernetes.CRDGroup, c.config.Kubernetes.CRDVersion),
			"kind":       "UAVMetrics",
			"spec":       spec,
		},
	}, nil
}

// ListUAVMetrics 列出所有UAVMetrics资源
func (c *Client) ListUAVMetrics(ctx context.Context) ([]*models.UAVMetrics, error) {
	// 获取所有UAVMetrics资源
	list, err := c.dynamicClient.Resource(c.gvr).
		Namespace(c.config.Kubernetes.Namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list UAVMetrics: %w", err)
	}

	// 转换为UAVMetrics切片
	metrics := make([]*models.UAVMetrics, 0, len(list.Items))
	for _, item := range list.Items {
		m, err := c.fromUnstructured(&item)
		if err != nil {
			// 记录错误但继续处理其他资源
			continue
		}
		metrics = append(metrics, m)
	}

	return metrics, nil
}

// fromUnstructured 将unstructured格式转换为UAVMetrics
func (c *Client) fromUnstructured(obj *unstructured.Unstructured) (*models.UAVMetrics, error) {
	// 获取spec字段
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil {
		return nil, fmt.Errorf("failed to get spec: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("spec not found")
	}

	// 转换为JSON再反序列化
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec: %w", err)
	}

	var metrics models.UAVMetrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to UAVMetrics: %w", err)
	}

	return &metrics, nil
}

// Clientset 返回Kubernetes clientset
func (c *Client) Clientset() *kubernetes.Clientset {
	return c.clientset
}

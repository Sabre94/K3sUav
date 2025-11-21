package istio

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/k3suav/uav-monitor/pkg/router"
	"istio.io/client-go/pkg/apis/networking/v1beta1"
	versionedclient "istio.io/client-go/pkg/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Reconciler 路由规则协调器
// 监听 UAVMetrics 和 Service 变化，自动同步 DestinationRule
type Reconciler struct {
	routerAgent *router.RouterAgent
	manager     *Manager
	converter   *Converter
	k8sClient   kubernetes.Interface
	options     *ReconcileOptions
	log         *logrus.Logger

	// 缓存上次的路由配置，用于判断是否需要更新
	cacheMutex sync.RWMutex
	cache      map[string][]byte // key: namespace/service
}

// NewReconciler 创建协调器
func NewReconciler(
	routerAgent *router.RouterAgent,
	istioClient versionedclient.Interface,
	k8sClient kubernetes.Interface,
	options *ReconcileOptions,
	log *logrus.Logger,
) *Reconciler {
	if options == nil {
		options = DefaultReconcileOptions()
	}

	return &Reconciler{
		routerAgent: routerAgent,
		manager:     NewManager(istioClient, log),
		converter:   NewConverter(options),
		k8sClient:   k8sClient,
		options:     options,
		log:         log,
		cache:       make(map[string][]byte),
	}
}

// Start 启动协调器
func (r *Reconciler) Start(ctx context.Context) error {
	if !r.options.Enabled {
		r.log.Info("Istio integration is disabled")
		return nil
	}

	r.log.Info("Starting Istio Reconciler")

	// 启动定期同步
	go r.reconcileLoop(ctx)

	return nil
}

// reconcileLoop 定期同步循环
func (r *Reconciler) reconcileLoop(ctx context.Context) {
	// 初始化时先执行一次全量同步
	r.reconcileAll(ctx)

	ticker := time.NewTicker(30 * time.Second) // 每 30 秒同步一次
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.log.Info("Reconciler stopped")
			return
		case <-ticker.C:
			r.reconcileAll(ctx)
		}
	}
}

// reconcileAll 全量同步所有服务的路由规则
func (r *Reconciler) reconcileAll(ctx context.Context) {
	r.log.Debug("Starting full reconciliation")

	// 获取所有 K8s Services
	services, err := r.k8sClient.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		r.log.WithError(err).Warn("Failed to list services")
		return
	}

	reconcileCount := 0
	errorCount := 0

	for _, svc := range services.Items {
		// 跳过没有 selector 的服务（如 kube-dns）
		if len(svc.Spec.Selector) == 0 {
			continue
		}

		// 跳过 headless service
		if svc.Spec.ClusterIP == "None" {
			continue
		}

		serviceName := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)

		// 调用 RouterAgent 计算路由权重
		weights, err := r.routerAgent.ComputeRouting(ctx, serviceName)
		if err != nil {
			r.log.WithError(err).WithField("service", serviceName).Debug("Failed to compute routing")
			continue
		}

		if len(weights) == 0 {
			r.log.WithField("service", serviceName).Debug("No endpoints for service")
			continue
		}

		// 创建路由配置
		config := &ServiceRoutingConfig{
			ServiceName: svc.Name,
			Namespace:   svc.Namespace,
			Endpoints:   weights,
		}

		// 同步到 Istio
		if err := r.reconcileService(ctx, config); err != nil {
			r.log.WithError(err).WithField("service", serviceName).Warn("Failed to reconcile service")
			errorCount++
		} else {
			reconcileCount++
		}
	}

	r.log.WithFields(logrus.Fields{
		"reconciled": reconcileCount,
		"errors":     errorCount,
	}).Debug("Full reconciliation completed")
}

// reconcileService 同步单个服务的路由规则
func (r *Reconciler) reconcileService(ctx context.Context, config *ServiceRoutingConfig) error {
	serviceName := fmt.Sprintf("%s/%s", config.Namespace, config.ServiceName)

	// 转换为 DestinationRule
	dr, err := r.converter.ConvertToDestinationRule(config)
	if err != nil {
		return fmt.Errorf("failed to convert to DestinationRule: %w", err)
	}

	// 检查是否需要更新
	if !r.shouldUpdate(serviceName, dr) {
		r.log.WithField("service", serviceName).Debug("No update needed")
		return nil
	}

	// 应用 DestinationRule
	if err := r.manager.Apply(ctx, dr); err != nil {
		return fmt.Errorf("failed to apply DestinationRule: %w", err)
	}

	// 更新缓存
	r.updateCache(serviceName, dr)

	return nil
}

// shouldUpdate 判断是否需要更新
// 通过比较新旧 DestinationRule 的 hash 来判断
func (r *Reconciler) shouldUpdate(serviceName string, newDR *v1beta1.DestinationRule) bool {
	r.cacheMutex.RLock()
	defer r.cacheMutex.RUnlock()

	// 简化版本：总是更新
	// 实际生产中可以计算 DR 的 hash 并比较
	return true
}

// updateCache 更新缓存
func (r *Reconciler) updateCache(serviceName string, dr *v1beta1.DestinationRule) {
	r.cacheMutex.Lock()
	defer r.cacheMutex.Unlock()

	// 简化版本：只存储时间戳
	r.cache[serviceName] = []byte(time.Now().Format(time.RFC3339))
}

// ReconcileService 手动触发单个服务的同步（供外部调用）
func (r *Reconciler) ReconcileService(ctx context.Context, namespace, serviceName string) error {
	fullServiceName := fmt.Sprintf("%s/%s", namespace, serviceName)

	weights, err := r.routerAgent.ComputeRouting(ctx, fullServiceName)
	if err != nil {
		return fmt.Errorf("failed to compute routing: %w", err)
	}

	config := &ServiceRoutingConfig{
		ServiceName: serviceName,
		Namespace:   namespace,
		Endpoints:   weights,
	}

	return r.reconcileService(ctx, config)
}

// DeleteService 删除服务的 DestinationRule
func (r *Reconciler) DeleteService(ctx context.Context, namespace, serviceName string) error {
	drName := r.converter.generateName(serviceName)
	return r.manager.Delete(ctx, drName, namespace)
}

// GetStats 获取协调器统计信息
func (r *Reconciler) GetStats() map[string]interface{} {
	r.cacheMutex.RLock()
	defer r.cacheMutex.RUnlock()

	return map[string]interface{}{
		"enabled":         r.options.Enabled,
		"cached_services": len(r.cache),
		"min_weight":      r.options.MinWeightThreshold,
	}
}

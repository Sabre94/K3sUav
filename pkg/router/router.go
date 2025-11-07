package router

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/k3suav/uav-monitor/pkg/k8s"
	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/router/algorithm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// RouterAgent 智能路由代理
// 在每个节点上运行，维护本地 UAV metrics 缓存，
// 并根据可插拔算法为服务请求计算最优路由
type RouterAgent struct {
	nodeName      string
	k8sClientset  *kubernetes.Clientset
	uavClient     *k8s.Client
	algorithm     algorithm.RoutingAlgorithm
	log           *logrus.Logger

	// 本地缓存：存储所有节点的 UAV metrics（内存中）
	metricsCache  map[string]*models.UAVMetrics
	metricsMutex  sync.RWMutex

	// Endpoint 缓存：存储所有服务的 endpoints
	endpointsCache map[string][]algorithm.Endpoint // key: service name
	endpointsMutex sync.RWMutex
}

// NewRouterAgent 创建 Router Agent 实例
func NewRouterAgent(
	nodeName string,
	k8sClientset *kubernetes.Clientset,
	uavClient *k8s.Client,
	routingAlgorithm algorithm.RoutingAlgorithm,
	log *logrus.Logger,
) *RouterAgent {
	return &RouterAgent{
		nodeName:       nodeName,
		k8sClientset:   k8sClientset,
		uavClient:      uavClient,
		algorithm:      routingAlgorithm,
		log:            log,
		metricsCache:   make(map[string]*models.UAVMetrics),
		endpointsCache: make(map[string][]algorithm.Endpoint),
	}
}

// Start 启动 Router Agent
func (r *RouterAgent) Start(ctx context.Context) error {
	r.log.WithFields(logrus.Fields{
		"node":      r.nodeName,
		"algorithm": r.algorithm.Name(),
	}).Info("Starting Router Agent")

	// 启动 UAV metrics 缓存更新
	go r.watchUAVMetrics(ctx)

	// 启动 Endpoint 监听
	go r.watchEndpoints(ctx)

	// 等待初始缓存就绪
	if err := r.waitForCacheReady(ctx); err != nil {
		return fmt.Errorf("cache initialization failed: %w", err)
	}

	r.log.Info("Router Agent started successfully")
	return nil
}

// watchUAVMetrics 监听并缓存所有节点的 UAV metrics
func (r *RouterAgent) watchUAVMetrics(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second) // 每 2 秒更新一次缓存
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics, err := r.uavClient.ListUAVMetrics(ctx)
			if err != nil {
				r.log.WithError(err).Warn("Failed to list UAV metrics")
				continue
			}

			// 更新缓存
			r.metricsMutex.Lock()
			r.metricsCache = make(map[string]*models.UAVMetrics)
			for _, m := range metrics {
				r.metricsCache[m.NodeName] = m
			}
			r.metricsMutex.Unlock()

			r.log.WithField("count", len(metrics)).Debug("UAV metrics cache updated")
		}
	}
}

// watchEndpoints 监听所有服务的 endpoints 变化
func (r *RouterAgent) watchEndpoints(ctx context.Context) {
	// 使用 informer 监听 endpoints 和 pods
	factory := informers.NewSharedInformerFactory(r.k8sClientset, 30*time.Second)

	// Pod informer
	podInformer := factory.Core().V1().Pods().Informer()
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.handlePodEvent,
		UpdateFunc: func(old, new interface{}) { r.handlePodEvent(new) },
		DeleteFunc: r.handlePodEvent,
	})

	// Endpoints informer
	endpointsInformer := factory.Core().V1().Endpoints().Informer()
	endpointsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.handleEndpointsEvent,
		UpdateFunc: func(old, new interface{}) { r.handleEndpointsEvent(new) },
		DeleteFunc: r.handleEndpointsEvent,
	})

	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())
}

// handlePodEvent 处理 Pod 事件
func (r *RouterAgent) handlePodEvent(obj interface{}) {
	// 当 Pod 变化时，重新构建 endpoints 缓存
	r.rebuildEndpointsCache(context.Background())
}

// handleEndpointsEvent 处理 Endpoints 事件
func (r *RouterAgent) handleEndpointsEvent(obj interface{}) {
	r.rebuildEndpointsCache(context.Background())
}

// rebuildEndpointsCache 重新构建 endpoints 缓存
func (r *RouterAgent) rebuildEndpointsCache(ctx context.Context) {
	// 获取所有 pods
	pods, err := r.k8sClientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		r.log.WithError(err).Warn("Failed to list pods")
		return
	}

	// 获取所有 endpoints
	endpointsList, err := r.k8sClientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})
	if err != nil {
		r.log.WithError(err).Warn("Failed to list endpoints")
		return
	}

	// 构建 Pod -> Node 映射
	podToNode := make(map[string]string)
	for _, pod := range pods.Items {
		podToNode[pod.Namespace+"/"+pod.Name] = pod.Spec.NodeName
	}

	// 重建缓存
	newCache := make(map[string][]algorithm.Endpoint)

	for _, ep := range endpointsList.Items {
		serviceName := ep.Namespace + "/" + ep.Name
		endpoints := make([]algorithm.Endpoint, 0)

		for _, subset := range ep.Subsets {
			for _, addr := range subset.Addresses {
				if addr.TargetRef == nil || addr.TargetRef.Kind != "Pod" {
					continue
				}

				podKey := ep.Namespace + "/" + addr.TargetRef.Name
				nodeName := podToNode[podKey]

				for _, port := range subset.Ports {
					endpoints = append(endpoints, algorithm.Endpoint{
						PodName:   addr.TargetRef.Name,
						PodIP:     addr.IP,
						NodeName:  nodeName,
						Namespace: ep.Namespace,
						Service:   ep.Name,
						Port:      port.Port,
					})
				}
			}
		}

		if len(endpoints) > 0 {
			newCache[serviceName] = endpoints
		}
	}

	// 更新缓存
	r.endpointsMutex.Lock()
	r.endpointsCache = newCache
	r.endpointsMutex.Unlock()

	r.log.WithField("services", len(newCache)).Debug("Endpoints cache updated")
}

// ComputeRouting 计算指定服务的路由权重
// 这是核心方法，本地查询缓存（无网络延迟）
func (r *RouterAgent) ComputeRouting(ctx context.Context, serviceName string) ([]algorithm.EndpointWeight, error) {
	// 从缓存获取源节点指标（本地查询）
	r.metricsMutex.RLock()
	sourceMetrics := r.metricsCache[r.nodeName]
	targetMetrics := make(map[string]*models.UAVMetrics)
	for k, v := range r.metricsCache {
		targetMetrics[k] = v
	}
	r.metricsMutex.RUnlock()

	if sourceMetrics == nil {
		return nil, fmt.Errorf("source node %s metrics not found in cache", r.nodeName)
	}

	// 从缓存获取目标 endpoints（本地查询）
	r.endpointsMutex.RLock()
	endpoints, exists := r.endpointsCache[serviceName]
	r.endpointsMutex.RUnlock()

	if !exists || len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints found for service %s", serviceName)
	}

	// 调用算法计算权重（本地计算）
	weights, err := r.algorithm.ComputeWeights(ctx, r.nodeName, sourceMetrics, endpoints, targetMetrics)
	if err != nil {
		return nil, fmt.Errorf("algorithm %s failed: %w", r.algorithm.Name(), err)
	}

	r.log.WithFields(logrus.Fields{
		"service":   serviceName,
		"algorithm": r.algorithm.Name(),
		"endpoints": len(weights),
	}).Debug("Routing computed")

	return weights, nil
}

// waitForCacheReady 等待缓存初始化完成
func (r *RouterAgent) waitForCacheReady(ctx context.Context) error {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("cache initialization timeout")
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.metricsMutex.RLock()
			metricsReady := len(r.metricsCache) > 0
			r.metricsMutex.RUnlock()

			if metricsReady {
				r.log.Info("Cache ready")
				return nil
			}
		}
	}
}

// GetCacheStats 获取缓存统计（用于调试）
func (r *RouterAgent) GetCacheStats() map[string]interface{} {
	r.metricsMutex.RLock()
	metricsCount := len(r.metricsCache)
	r.metricsMutex.RUnlock()

	r.endpointsMutex.RLock()
	servicesCount := len(r.endpointsCache)
	r.endpointsMutex.RUnlock()

	return map[string]interface{}{
		"metrics_cached":  metricsCount,
		"services_cached": servicesCount,
		"node_name":       r.nodeName,
		"algorithm":       r.algorithm.Name(),
	}
}

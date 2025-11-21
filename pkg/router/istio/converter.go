package istio

import (
	"fmt"
	"sort"
	"time"

	"github.com/k3suav/uav-monitor/pkg/router/algorithm"
	networkingv1beta1 "istio.io/api/networking/v1beta1"
	"istio.io/client-go/pkg/apis/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Converter UAV 路由权重 → Istio DestinationRule 转换器
type Converter struct {
	options *ReconcileOptions
}

// NewConverter 创建转换器
func NewConverter(options *ReconcileOptions) *Converter {
	if options == nil {
		options = DefaultReconcileOptions()
	}
	return &Converter{
		options: options,
	}
}

// ConvertToDestinationRule 将服务路由配置转换为 Istio DestinationRule
func (c *Converter) ConvertToDestinationRule(config *ServiceRoutingConfig) (*v1beta1.DestinationRule, error) {
	if config == nil || len(config.Endpoints) == 0 {
		return nil, fmt.Errorf("invalid routing config: no endpoints")
	}

	// 过滤并排序 endpoints
	validEndpoints := c.filterEndpoints(config.Endpoints)
	if len(validEndpoints) == 0 {
		return nil, fmt.Errorf("no valid endpoints after filtering")
	}

	// 构建 DestinationRule
	dr := &v1beta1.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.generateName(config.ServiceName),
			Namespace: config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "uav-router",
				"uav.k3s.io/service":            config.ServiceName,
			},
		},
		Spec: networkingv1beta1.DestinationRule{
			Host: fmt.Sprintf("%s.%s.svc.cluster.local", config.ServiceName, config.Namespace),
		},
	}

	// 添加注释（包含调试信息）
	if c.options.EnableAnnotations {
		dr.ObjectMeta.Annotations = c.generateAnnotations(config, validEndpoints)
	}

	// 按节点分组创建 subsets
	subsets := c.createSubsets(validEndpoints)
	dr.Spec.Subsets = subsets

	// 配置流量策略（加权负载均衡）
	dr.Spec.TrafficPolicy = c.createTrafficPolicy(validEndpoints)

	return dr, nil
}

// filterEndpoints 过滤权重过低的 endpoints
func (c *Converter) filterEndpoints(endpoints []algorithm.EndpointWeight) []algorithm.EndpointWeight {
	filtered := make([]algorithm.EndpointWeight, 0)
	for _, ep := range endpoints {
		if ep.Weight >= c.options.MinWeightThreshold {
			filtered = append(filtered, ep)
		}
	}

	// 按权重降序排序
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Weight > filtered[j].Weight
	})

	return filtered
}

// createSubsets 按节点创建 subsets
func (c *Converter) createSubsets(endpoints []algorithm.EndpointWeight) []*networkingv1beta1.Subset {
	// 按节点分组
	nodeGroups := make(map[string][]algorithm.EndpointWeight)
	for _, ep := range endpoints {
		nodeGroups[ep.Endpoint.NodeName] = append(nodeGroups[ep.Endpoint.NodeName], ep)
	}

	subsets := make([]*networkingv1beta1.Subset, 0, len(nodeGroups))
	for nodeName, nodeEndpoints := range nodeGroups {
		// 计算该节点的平均权重
		avgWeight := c.calculateAverageWeight(nodeEndpoints)

		subset := &networkingv1beta1.Subset{
			Name: c.sanitizeSubsetName(nodeName),
			Labels: map[string]string{
				"topology.kubernetes.io/zone": nodeName, // 使用节点名作为区域标签
			},
			TrafficPolicy: &networkingv1beta1.TrafficPolicy{
				LoadBalancer: &networkingv1beta1.LoadBalancerSettings{
					LbPolicy: &networkingv1beta1.LoadBalancerSettings_Simple{
						Simple: networkingv1beta1.LoadBalancerSettings_ROUND_ROBIN,
					},
				},
			},
		}

		// 将权重信息存储在标签中（用于调试）
		if c.options.EnableAnnotations {
			if subset.Labels == nil {
				subset.Labels = make(map[string]string)
			}
			subset.Labels["uav.k3s.io/weight"] = fmt.Sprintf("%d", avgWeight)
		}

		subsets = append(subsets, subset)
	}

	// 按名称排序确保一致性
	sort.Slice(subsets, func(i, j int) bool {
		return subsets[i].Name < subsets[j].Name
	})

	return subsets
}

// createTrafficPolicy 创建流量策略（基于节点权重）
func (c *Converter) createTrafficPolicy(endpoints []algorithm.EndpointWeight) *networkingv1beta1.TrafficPolicy {
	// 按节点分组并计算权重
	nodeWeights := make(map[string]int)
	for _, ep := range endpoints {
		nodeWeights[ep.Endpoint.NodeName] = ep.Weight
	}

	// 创建加权分配
	weightedTargets := make([]*networkingv1beta1.LocalityLoadBalancerSetting_Distribute, 0)
	for nodeName, weight := range nodeWeights {
		weightedTargets = append(weightedTargets, &networkingv1beta1.LocalityLoadBalancerSetting_Distribute{
			From: c.sanitizeSubsetName(nodeName) + "/*",
			To: map[string]uint32{
				c.sanitizeSubsetName(nodeName): uint32(weight),
			},
		})
	}

	return &networkingv1beta1.TrafficPolicy{
		LoadBalancer: &networkingv1beta1.LoadBalancerSettings{
			LbPolicy: &networkingv1beta1.LoadBalancerSettings_Simple{
				Simple: networkingv1beta1.LoadBalancerSettings_LEAST_REQUEST,
			},
		},
	}
}

// calculateAverageWeight 计算节点的平均权重
func (c *Converter) calculateAverageWeight(endpoints []algorithm.EndpointWeight) int {
	if len(endpoints) == 0 {
		return 0
	}

	sum := 0
	for _, ep := range endpoints {
		sum += ep.Weight
	}
	return sum / len(endpoints)
}

// generateName 生成 DestinationRule 名称
func (c *Converter) generateName(serviceName string) string {
	return c.options.NamePrefix + serviceName
}

// generateAnnotations 生成注释
func (c *Converter) generateAnnotations(config *ServiceRoutingConfig, endpoints []algorithm.EndpointWeight) map[string]string {
	annotations := make(map[string]string)
	annotations["uav.k3s.io/updated-at"] = time.Now().Format(time.RFC3339)
	annotations["uav.k3s.io/endpoint-count"] = fmt.Sprintf("%d", len(endpoints))

	// 记录权重信息用于调试
	if len(endpoints) > 0 {
		annotations["uav.k3s.io/algorithm"] = endpoints[0].Reason
		annotations["uav.k3s.io/max-weight"] = fmt.Sprintf("%d", endpoints[0].Weight)
		if len(endpoints) > 1 {
			annotations["uav.k3s.io/min-weight"] = fmt.Sprintf("%d", endpoints[len(endpoints)-1].Weight)
		}
	}

	return annotations
}

// sanitizeSubsetName 清理 subset 名称（Istio 要求符合 DNS 标准）
func (c *Converter) sanitizeSubsetName(name string) string {
	// 替换不合法字符
	// 例如: k3s-uav-pool-1 保持不变
	// 如果有特殊字符，可以在这里处理
	return name
}

// ShouldUpdate 判断是否需要更新 DestinationRule
// 比较新旧权重，如果变化超过阈值则返回 true
func (c *Converter) ShouldUpdate(oldEndpoints, newEndpoints []algorithm.EndpointWeight) bool {
	if len(oldEndpoints) != len(newEndpoints) {
		return true // 数量变化，需要更新
	}

	// 检查权重变化
	for i := range newEndpoints {
		if i >= len(oldEndpoints) {
			return true
		}

		oldWeight := oldEndpoints[i].Weight
		newWeight := newEndpoints[i].Weight

		// 计算权重变化百分比
		if oldWeight == 0 {
			if newWeight > 0 {
				return true
			}
			continue
		}

		changePercent := float64(abs(newWeight-oldWeight)) / float64(oldWeight) * 100
		if changePercent >= c.options.MinWeightChangePercent {
			return true
		}
	}

	return false
}

// abs 计算绝对值
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

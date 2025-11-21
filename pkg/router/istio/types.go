package istio

import (
	"github.com/k3suav/uav-monitor/pkg/router/algorithm"
)

// ServiceRoutingConfig 服务的路由配置
// 包含服务名称、命名空间、以及每个 endpoint 的权重
type ServiceRoutingConfig struct {
	ServiceName string
	Namespace   string
	Endpoints   []algorithm.EndpointWeight
}

// DestinationRuleConfig DestinationRule 配置
// 用于生成 Istio DestinationRule CR
type DestinationRuleConfig struct {
	Name        string
	Namespace   string
	Host        string
	Subsets     []SubsetConfig
	UpdatedAt   string
}

// SubsetConfig Subset 配置
// 每个 subset 对应一个节点，包含权重信息
type SubsetConfig struct {
	Name      string            // subset 名称，通常是节点名
	Labels    map[string]string // 用于选择 Pod 的标签
	Weight    int               // 路由权重 (0-100)
	Priority  int               // 优先级
}

// ReconcileOptions Reconciler 配置选项
type ReconcileOptions struct {
	// 是否启用 Istio 集成
	Enabled bool

	// 最小权重阈值，低于此值的 endpoint 不会包含在 DestinationRule 中
	MinWeightThreshold int

	// 最小权重变化百分比，低于此值不会触发更新
	MinWeightChangePercent float64

	// 是否在 DestinationRule 中添加注释（包含调试信息）
	EnableAnnotations bool

	// DestinationRule 名称前缀
	NamePrefix string
}

// DefaultReconcileOptions 返回默认配置
func DefaultReconcileOptions() *ReconcileOptions {
	return &ReconcileOptions{
		Enabled:                true,
		MinWeightThreshold:     10, // 权重低于 10 的不包含
		MinWeightChangePercent: 5.0, // 权重变化超过 5% 才更新
		EnableAnnotations:      true,
		NamePrefix:             "uav-",
	}
}

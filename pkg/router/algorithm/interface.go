package algorithm

import (
	"context"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// RoutingAlgorithm 定义路由算法接口
// 所有路由算法必须实现此接口才能被 Router Agent 使用
type RoutingAlgorithm interface {
	// Name 返回算法名称
	Name() string

	// ComputeWeights 根据源节点和目标 endpoints，计算路由权重
	// sourceNode: 调用方所在的节点名
	// sourceMetrics: 调用方节点的 UAV 指标（GPS、电量等）
	// targetEndpoints: 目标服务的所有 Pod endpoints
	// targetMetrics: 所有目标节点的 UAV 指标（key 是 nodeName）
	// 返回: 每个 endpoint 的权重列表，权重越高越优先路由
	ComputeWeights(
		ctx context.Context,
		sourceNode string,
		sourceMetrics *models.UAVMetrics,
		targetEndpoints []Endpoint,
		targetMetrics map[string]*models.UAVMetrics,
	) ([]EndpointWeight, error)
}

// Endpoint 表示一个服务的 endpoint（Pod）
type Endpoint struct {
	PodName   string // Pod 名称
	PodIP     string // Pod IP 地址
	NodeName  string // Pod 所在节点
	Namespace string // Pod 命名空间
	Service   string // 所属服务名
	Port      int32  // 服务端口
}

// EndpointWeight 表示 endpoint 的路由权重
type EndpointWeight struct {
	Endpoint Endpoint // 目标 endpoint
	Weight   int      // 权重 (0-100)，越高越优先
	Priority int      // 优先级 (0 最高)，相同优先级内按权重分配
	Reason   string   // 选择原因（用于调试和日志）
}

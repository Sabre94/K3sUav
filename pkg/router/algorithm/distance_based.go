package algorithm

import (
	"context"
	"fmt"
	"math"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// DistanceBasedRouter 基于地理距离的路由算法
// 优先将流量路由到地理距离最近的 Pod
type DistanceBasedRouter struct {
	// MaxDistance 最大可接受距离（公里），超过此距离的节点将被过滤
	MaxDistance float64
}

// NewDistanceBasedRouter 创建基于距离的路由算法实例
func NewDistanceBasedRouter(maxDistance float64) *DistanceBasedRouter {
	if maxDistance <= 0 {
		maxDistance = 1000.0 // 默认最大 1000 公里
	}
	return &DistanceBasedRouter{
		MaxDistance: maxDistance,
	}
}

// Name 返回算法名称
func (r *DistanceBasedRouter) Name() string {
	return "distance-based"
}

// ComputeWeights 计算基于距离的路由权重
func (r *DistanceBasedRouter) ComputeWeights(
	ctx context.Context,
	sourceNode string,
	sourceMetrics *models.UAVMetrics,
	targetEndpoints []Endpoint,
	targetMetrics map[string]*models.UAVMetrics,
) ([]EndpointWeight, error) {

	if sourceMetrics == nil {
		return nil, fmt.Errorf("source metrics is nil for node %s", sourceNode)
	}

	weights := make([]EndpointWeight, 0, len(targetEndpoints))

	for _, ep := range targetEndpoints {
		// 获取目标节点的指标
		targetM, exists := targetMetrics[ep.NodeName]
		if !exists {
			// 如果没有目标节点的指标，跳过此 endpoint
			continue
		}

		// 计算两点之间的地理距离
		distance := CalculateDistance(
			sourceMetrics.GPS.Latitude,
			sourceMetrics.GPS.Longitude,
			targetM.GPS.Latitude,
			targetM.GPS.Longitude,
		)

		// 距离过滤：超过最大距离的节点不参与路由
		if distance > r.MaxDistance {
			continue
		}

		// 计算权重：距离越近权重越高
		// 使用指数衰减公式：weight = 100 * e^(-distance/scale)
		// 这样可以让距离差异的影响更平滑
		scale := 50.0 // 衰减尺度（公里）
		weight := 100.0 * math.Exp(-distance/scale)

		// 确保权重在 1-100 范围内
		if weight < 1 {
			weight = 1
		}

		weights = append(weights, EndpointWeight{
			Endpoint: ep,
			Weight:   int(weight),
			Priority: 0, // 所有节点同等优先级
			Reason:   fmt.Sprintf("distance: %.2fkm", distance),
		})
	}

	if len(weights) == 0 {
		return nil, fmt.Errorf("no eligible endpoints found within %.2f km", r.MaxDistance)
	}

	return weights, nil
}

// CalculateDistance 计算两个 GPS 坐标之间的距离（公里）
// 使用 Haversine 公式计算地球表面两点之间的大圆距离
func CalculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0

	// 将角度转换为弧度
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	// Haversine 公式
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

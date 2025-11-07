package algorithm

import (
	"context"
	"fmt"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// CompositeRouter 组合路由算法
// 结合多个算法，根据权重计算最终路由决策
type CompositeRouter struct {
	// Algorithms 子算法列表
	Algorithms []RoutingAlgorithm
	// Weights 每个算法的权重（对应 Algorithms 列表）
	Weights []float64
}

// NewCompositeRouter 创建组合路由算法实例
func NewCompositeRouter(algorithms []RoutingAlgorithm, weights []float64) (*CompositeRouter, error) {
	if len(algorithms) != len(weights) {
		return nil, fmt.Errorf("algorithms and weights length mismatch")
	}
	if len(algorithms) == 0 {
		return nil, fmt.Errorf("at least one algorithm required")
	}

	// 归一化权重
	var sum float64
	for _, w := range weights {
		sum += w
	}
	normalizedWeights := make([]float64, len(weights))
	for i, w := range weights {
		normalizedWeights[i] = w / sum
	}

	return &CompositeRouter{
		Algorithms: algorithms,
		Weights:    normalizedWeights,
	}, nil
}

// Name 返回算法名称
func (r *CompositeRouter) Name() string {
	return "composite"
}

// ComputeWeights 计算组合路由权重
func (r *CompositeRouter) ComputeWeights(
	ctx context.Context,
	sourceNode string,
	sourceMetrics *models.UAVMetrics,
	targetEndpoints []Endpoint,
	targetMetrics map[string]*models.UAVMetrics,
) ([]EndpointWeight, error) {

	// 存储每个 endpoint 的累积权重
	totalScores := make(map[string]float64)
	reasonMap := make(map[string][]string)

	// 对每个算法计算权重，然后加权求和
	for i, algo := range r.Algorithms {
		weights, err := algo.ComputeWeights(ctx, sourceNode, sourceMetrics, targetEndpoints, targetMetrics)
		if err != nil {
			// 如果某个算法失败，记录但继续其他算法
			continue
		}

		for _, w := range weights {
			key := w.Endpoint.PodIP // 使用 Pod IP 作为唯一标识
			totalScores[key] += float64(w.Weight) * r.Weights[i]
			reasonMap[key] = append(reasonMap[key],
				fmt.Sprintf("%s(%.0f%%): %s",
					algo.Name(),
					r.Weights[i]*100,
					w.Reason))
		}
	}

	if len(totalScores) == 0 {
		return nil, fmt.Errorf("no eligible endpoints found by any algorithm")
	}

	// 转换为 EndpointWeight 列表
	weights := make([]EndpointWeight, 0, len(totalScores))
	endpointMap := make(map[string]Endpoint)

	// 构建 endpoint map
	for _, ep := range targetEndpoints {
		endpointMap[ep.PodIP] = ep
	}

	for podIP, score := range totalScores {
		ep, exists := endpointMap[podIP]
		if !exists {
			continue
		}

		// 确保权重在 1-100 范围内
		finalWeight := score
		if finalWeight > 100 {
			finalWeight = 100
		}
		if finalWeight < 1 {
			finalWeight = 1
		}

		weights = append(weights, EndpointWeight{
			Endpoint: ep,
			Weight:   int(finalWeight),
			Priority: 0,
			Reason:   fmt.Sprintf("composite: %v", reasonMap[podIP]),
		})
	}

	return weights, nil
}

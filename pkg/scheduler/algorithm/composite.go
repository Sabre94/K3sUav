package algorithm

import (
	"context"
	"fmt"

	"github.com/k3suav/uav-monitor/pkg/models"
	v1 "k8s.io/api/core/v1"
)

// CompositeAlgorithm 组合算法
// 将多个算法的结果按权重合并
type CompositeAlgorithm struct {
	Algorithms []SchedulingAlgorithm // 子算法列表
	Weights    []float64              // 对应的权重
}

// NewCompositeAlgorithm 创建组合算法
func NewCompositeAlgorithm(algorithms []SchedulingAlgorithm, weights []float64) *CompositeAlgorithm {
	// 如果权重数量不匹配，自动补充为平均权重
	if len(weights) != len(algorithms) {
		weights = make([]float64, len(algorithms))
		avgWeight := 1.0 / float64(len(algorithms))
		for i := range weights {
			weights[i] = avgWeight
		}
	}

	// 归一化权重（使总和为1）
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	if sum > 0 {
		for i := range weights {
			weights[i] /= sum
		}
	}

	return &CompositeAlgorithm{
		Algorithms: algorithms,
		Weights:    weights,
	}
}

func (a *CompositeAlgorithm) Name() string {
	return "composite"
}

func (a *CompositeAlgorithm) Filter(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]*models.UAVMetrics, error) {
	// 依次应用所有子算法的过滤器
	filtered := metrics
	for _, algo := range a.Algorithms {
		var err error
		filtered, err = algo.Filter(ctx, pod, filtered)
		if err != nil {
			return nil, fmt.Errorf("filter error in %s: %w", algo.Name(), err)
		}
	}
	return filtered, nil
}

func (a *CompositeAlgorithm) Score(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]NodeScore, error) {
	// 存储每个节点的加权总分
	totalScores := make(map[string]float64)
	reasons := make(map[string][]string)

	// 计算每个算法的分数并加权
	for i, algo := range a.Algorithms {
		scores, err := algo.Score(ctx, pod, metrics)
		if err != nil {
			return nil, fmt.Errorf("score error in %s: %w", algo.Name(), err)
		}

		// 累加加权分数
		for _, s := range scores {
			totalScores[s.NodeName] += s.Score * a.Weights[i]
			reasons[s.NodeName] = append(reasons[s.NodeName],
				fmt.Sprintf("%s(%.0f%%, score:%.1f)", algo.Name(), a.Weights[i]*100, s.Score))
		}
	}

	// 转换为结果
	result := []NodeScore{}
	for node, score := range totalScores {
		result = append(result, NodeScore{
			NodeName: node,
			Score:    score,
			Reason:   fmt.Sprintf("composite: %v", reasons[node]),
		})
	}

	return result, nil
}

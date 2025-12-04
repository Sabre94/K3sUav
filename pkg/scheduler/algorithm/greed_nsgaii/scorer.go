package greed_nsgaii

import (
	"math"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// NodeScorer 节点评分器
type NodeScorer struct {
	taskType TaskType
	weights  TaskWeights
}

// NewNodeScorer 创建节点评分器
func NewNodeScorer(taskType TaskType) *NodeScorer {
	return &NodeScorer{
		taskType: taskType,
		weights:  GetTaskWeights(taskType),
	}
}

// CalculateScore 计算节点的综合得分（0-100）
func (s *NodeScorer) CalculateScore(metrics *models.UAVMetrics) float64 {
	// 1. 电量分数（归一化到 0-100）
	batteryScore := normalizeValue(metrics.Battery.RemainingPercent, 0, 100, false)

	// 2. 延迟分数（归一化到 0-100，延迟越低越好）
	latencyScore := normalizeValue(metrics.Network.Latency, 10, 150, true)

	// 3. 利用率分数（归一化到 0-100，利用率越低越好）
	// 综合 CPU 和内存利用率
	avgUtil := (metrics.Performance.CPUUsage + metrics.Performance.MemoryUsage) / 2.0
	utilScore := normalizeValue(avgUtil, 5, 80, true)

	// 4. 加权求和
	finalScore := s.weights.Battery*batteryScore +
		s.weights.Latency*latencyScore +
		s.weights.Util*utilScore

	return finalScore
}

// CalculateScores 批量计算所有节点的分数
func (s *NodeScorer) CalculateScores(allMetrics []*models.UAVMetrics) []*NodeInfo {
	nodeInfos := make([]*NodeInfo, len(allMetrics))

	for i, metrics := range allMetrics {
		nodeInfos[i] = &NodeInfo{
			Metrics: metrics,
			Score:   s.CalculateScore(metrics),
		}
	}

	return nodeInfos
}

// normalizeValue 归一化值到 0-100 区间
// reverse=true 表示值越小越好（如延迟、利用率）
func normalizeValue(value, min, max float64, reverse bool) float64 {
	// 限制在 [min, max] 范围内
	if value < min {
		value = min
	}
	if value > max {
		value = max
	}

	// 归一化到 [0, 1]
	normalized := (value - min) / (max - min)

	// 如果是反向指标（越小越好），取反
	if reverse {
		normalized = 1.0 - normalized
	}

	// 缩放到 [0, 100]
	return normalized * 100.0
}

// CalculateUtilization 计算节点的综合利用率（0-100）
func CalculateUtilization(metrics *models.UAVMetrics) float64 {
	return (metrics.Performance.CPUUsage + metrics.Performance.MemoryUsage) / 2.0
}

// CalculateMultiObjectives 计算多目标优化的目标值
// 返回：[负平均电量, 平均延迟, 平均利用率, UAV数量]
func CalculateMultiObjectives(selectedNodes []*NodeInfo) []float64 {
	if len(selectedNodes) == 0 {
		return []float64{0, 0, 0, 0}
	}

	var totalBattery, totalLatency, totalUtil float64
	for _, node := range selectedNodes {
		totalBattery += node.Metrics.Battery.RemainingPercent
		totalLatency += node.Metrics.Network.Latency
		totalUtil += CalculateUtilization(node.Metrics)
	}

	n := float64(len(selectedNodes))
	avgBattery := totalBattery / n
	avgLatency := totalLatency / n
	avgUtil := totalUtil / n

	// 目标1: 最大化平均电量 -> 最小化负平均电量
	obj1 := -avgBattery

	// 目标2: 最小化平均延迟
	obj2 := avgLatency

	// 目标3: 最小化平均利用率
	obj3 := avgUtil

	// 目标4: 最小化 UAV 数量
	obj4 := n

	return []float64{obj1, obj2, obj3, obj4}
}

// Dominates 判断个体 a 是否支配个体 b（Pareto 支配关系）
// 使用约束支配（Constrained Dominance）：优先考虑可行性
func Dominates(a, b *Individual) bool {
	// 规则1: 可行解支配不可行解
	if a.IsFeasible && !b.IsFeasible {
		return true
	}
	if !a.IsFeasible && b.IsFeasible {
		return false
	}

	// 规则2: 两者都不可行时，不进行支配比较（都是平等的）
	if !a.IsFeasible && !b.IsFeasible {
		return false
	}

	// 规则3: 两者都可行时，进行 Pareto 支配比较
	// a 支配 b 当且仅当：
	// - a 在所有目标上不劣于 b
	// - a 在至少一个目标上严格优于 b

	betterInAny := false
	for i := 0; i < len(a.Objectives); i++ {
		if a.Objectives[i] > b.Objectives[i] {
			// a 在目标 i 上劣于 b
			return false
		}
		if a.Objectives[i] < b.Objectives[i] {
			// a 在目标 i 上优于 b
			betterInAny = true
		}
	}

	return betterInAny
}

// CrowdingDistanceComparison 拥挤度比较（用于选择）
// 优先选择 rank 小的，rank 相同时选择拥挤度大的
func CrowdingDistanceComparison(a, b *Individual) bool {
	if a.Rank != b.Rank {
		return a.Rank < b.Rank
	}
	return a.CrowdingDistance > b.CrowdingDistance
}

// EuclideanDistanceObjectives 计算两个个体在目标空间中的欧几里得距离
func EuclideanDistanceObjectives(a, b *Individual) float64 {
	var sum float64
	for i := 0; i < len(a.Objectives); i++ {
		diff := a.Objectives[i] - b.Objectives[i]
		sum += diff * diff
	}
	return math.Sqrt(sum)
}

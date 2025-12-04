package greed_nsgaii

import (
	"github.com/k3suav/uav-monitor/pkg/models"
)

// NodeInfo 节点信息（包含 UAVMetrics 和计算得分）
type NodeInfo struct {
	Metrics *models.UAVMetrics
	Score   float64 // 综合得分（基于任务类型）
	XMeters float64 // 相对X坐标（米）
	YMeters float64 // 相对Y坐标（米）
}

// TaskType 任务类型
type TaskType string

const (
	TaskTypeEmergency TaskType = "emergency" // 紧急任务：优先考虑延迟和电量
	TaskTypeSustain   TaskType = "sustain"   // 持续任务：优先考虑电量
	TaskTypeCompute   TaskType = "compute"   // 计算任务：优先考虑CPU/内存利用率
	TaskTypeDefault   TaskType = "default"   // 默认任务：均衡考虑
)

// TaskWeights 任务类型对应的权重
type TaskWeights struct {
	Battery float64 // 电量权重
	Latency float64 // 延迟权重
	Util    float64 // 利用率权重（CPU + 内存）
}

// GetTaskWeights 获取任务类型对应的权重
func GetTaskWeights(taskType TaskType) TaskWeights {
	switch taskType {
	case TaskTypeEmergency:
		return TaskWeights{Battery: 0.4, Latency: 0.5, Util: 0.1}
	case TaskTypeSustain:
		return TaskWeights{Battery: 0.5, Latency: 0.2, Util: 0.3}
	case TaskTypeCompute:
		return TaskWeights{Battery: 0.2, Latency: 0.4, Util: 0.4}
	default:
		return TaskWeights{Battery: 0.33, Latency: 0.33, Util: 0.34}
	}
}

// CoverageConfig 覆盖率配置
type CoverageConfig struct {
	TargetCoverageRatio float64 // 目标覆盖率（0.0 - 1.0）
	CoverageRadius      float64 // 节点覆盖半径（米）
	GridDensity         int     // 网格密度（用于计算覆盖面积）
}

// DeploymentState Deployment 的调度状态
type DeploymentState struct {
	SelectedNodes       []string    // 已选择的节点名称列表
	SelectedNodesInfo   []*NodeInfo // 已选择节点的详细信息
	CurrentCoverageArea float64     // 当前覆盖面积（平方米）
	MaxPossibleArea     float64     // 最大可能覆盖面积（平方米）
}

// Individual NSGA-II 个体
type Individual struct {
	Chromosome       []bool    // 染色体：每个位表示是否选择该节点
	Objectives       []float64 // 目标值：[负平均电量, 平均延迟, 平均利用率, UAV数量]
	IsFeasible       bool      // 是否满足覆盖率约束
	Rank             int       // Pareto 等级
	CrowdingDistance float64   // 拥挤度距离
}

// Population NSGA-II 种群
type Population []*Individual

// GreedyResult 贪心算法结果
type GreedyResult struct {
	SelectedNodes       []*NodeInfo // 选中的节点
	FinalUnionArea      float64     // 最终覆盖面积（平方米）
	AchievedCoverage    float64     // 实际覆盖率（0.0 - 1.0）
	MaxPossibleArea     float64     // 最大可能覆盖面积（平方米）
}

// NSGA2Result NSGA-II 优化结果
type NSGA2Result struct {
	ParetoFront   Population  // Pareto 前沿（非支配解集）
	BestSolution  *Individual // 推荐的最佳解（基于某种策略）
	AllPopulation Population  // 所有种群
}

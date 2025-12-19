package rl_coverage

import (
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// NodeFeatures 节点特征向量
type NodeFeatures struct {
	// 位置特征 (归一化到 0-1)
	NormX float64 // 归一化 X 坐标
	NormY float64 // 归一化 Y 坐标

	// 资源特征 (归一化到 0-1)
	Battery     float64 // 电量百分比 / 100
	Latency     float64 // 延迟 (归一化)
	CPUUsage    float64 // CPU 使用率
	MemoryUsage float64 // 内存使用率

	// 覆盖特征
	DistanceToCenter  float64 // 到中心的距离 (归一化)
	NearestNeighborDist float64 // 到最近邻居的距离 (归一化)
}

// State RL 状态
type State struct {
	// 所有节点的特征
	NodeFeatures []NodeFeatures

	// 全局特征
	CurrentCoverage    float64 // 当前覆盖率
	SelectedCount      int     // 已选择节点数
	TotalNodes         int     // 总节点数
	TargetCoverage     float64 // 目标覆盖率

	// 选择掩码 (1=可选, 0=已选或不可用)
	SelectionMask []float64
}

// Action RL 动作
type Action struct {
	NodeIndex int     // 选择的节点索引 (-1 表示停止)
	Prob      float64 // 动作概率
}

// Experience 经验回放
type Experience struct {
	State     *State
	Action    Action
	Reward    float64
	NextState *State
	Done      bool
}

// Episode 一个完整的调度过程
type Episode struct {
	Experiences []Experience
	TotalReward float64
	FinalCoverage float64
	SelectedNodes int
}

// NodeInfo 节点信息 (用于内部计算)
type NodeInfo struct {
	Metrics  *models.UAVMetrics
	Features NodeFeatures
	XMeters  float64
	YMeters  float64
	Score    float64
}

// RLConfig 强化学习配置
type RLConfig struct {
	// 网络配置
	HiddenSize     int     // 隐藏层大小
	NumHiddenLayers int    // 隐藏层数量

	// 训练配置
	LearningRate   float64 // 学习率
	Gamma          float64 // 折扣因子
	EntropyCoef    float64 // 熵正则化系数
	ClipEpsilon    float64 // PPO clip 参数

	// 奖励配置
	CoverageRewardScale float64 // 覆盖率奖励缩放
	NodePenalty         float64 // 节点数惩罚
	BatteryBonus        float64 // 高电量奖励
	TargetBonus         float64 // 达到目标覆盖率奖励

	// 环境配置
	TargetCoverage float64 // 目标覆盖率
	CoverageRadius float64 // 覆盖半径 (米)
	GridDensity    int     // 网格密度

	// 训练参数
	BatchSize      int     // 批大小
	EpisodesPerUpdate int  // 每次更新的 episode 数
	MaxStepsPerEpisode int // 每个 episode 最大步数
}

// DefaultRLConfig 默认配置
func DefaultRLConfig() *RLConfig {
	return &RLConfig{
		// 网络 (较小的网络用于快速训练和推理)
		HiddenSize:      64,
		NumHiddenLayers: 1,

		// 训练
		LearningRate:   0.01,
		Gamma:          0.99,
		EntropyCoef:    0.01,
		ClipEpsilon:    0.2,

		// 奖励
		CoverageRewardScale: 10.0,
		NodePenalty:         0.1,
		BatteryBonus:        0.5,
		TargetBonus:         5.0,

		// 环境
		TargetCoverage: 0.90,
		CoverageRadius: 500.0,
		GridDensity:    20, // 比 NSGA-II 小，加快计算

		// 训练参数
		BatchSize:          32,
		EpisodesPerUpdate:  5,
		MaxStepsPerEpisode: 50,
	}
}

// TrainingStats 训练统计
type TrainingStats struct {
	Episode       int
	AvgReward     float64
	AvgCoverage   float64
	AvgNodes      float64
	Loss          float64
	Entropy       float64
	Duration      time.Duration
}

// ModelCheckpoint 模型检查点
type ModelCheckpoint struct {
	Weights   [][]float64 // 网络权重
	Config    *RLConfig   // 配置
	Stats     TrainingStats // 训练统计
	Timestamp time.Time
}

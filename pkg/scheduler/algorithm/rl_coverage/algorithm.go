package rl_coverage

import (
	"context"
	"fmt"
	"sync"

	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm"
	v1 "k8s.io/api/core/v1"
)

// RLCoverageAlgorithm 基于强化学习的覆盖率优化算法
type RLCoverageAlgorithm struct {
	config *RLConfig
	policy *PolicyNetwork
	env    *Environment

	// 状态缓存
	deploymentStates map[string]*DeploymentState
	mu               sync.RWMutex

	// 是否已训练
	isTrained bool

	// Deployment 锁
	deploymentLocks map[string]*sync.Mutex
	locksMu         sync.Mutex
}

// DeploymentState Deployment 状态
type DeploymentState struct {
	SelectedNodes []string
	Coverage      float64
}

// NewRLCoverageAlgorithm 创建 RL 覆盖率算法
func NewRLCoverageAlgorithm(config *RLConfig) *RLCoverageAlgorithm {
	if config == nil {
		config = DefaultRLConfig()
	}

	// 输入维度: 8 节点特征 + 4 全局特征 = 12
	inputSize := 12

	return &RLCoverageAlgorithm{
		config:           config,
		policy:           NewPolicyNetwork(inputSize, config.HiddenSize, config.NumHiddenLayers),
		env:              NewEnvironment(config),
		deploymentStates: make(map[string]*DeploymentState),
		deploymentLocks:  make(map[string]*sync.Mutex),
		isTrained:        false,
	}
}

// Name 返回算法名称
func (a *RLCoverageAlgorithm) Name() string {
	return "rl-coverage"
}

// SetTargetCoverage 动态设置目标覆盖率
func (a *RLCoverageAlgorithm) SetTargetCoverage(target float64) {
	a.config.TargetCoverage = target
	a.env.config.TargetCoverage = target
}

// Filter 过滤节点 (不做过滤)
func (a *RLCoverageAlgorithm) Filter(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]*models.UAVMetrics, error) {
	return metrics, nil
}

// Score 为每个节点计算分数
func (a *RLCoverageAlgorithm) Score(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]algorithm.NodeScore, error) {
	if len(metrics) == 0 {
		return nil, fmt.Errorf("no metrics available")
	}

	// 获取 Deployment 名称
	deploymentName := getDeploymentName(pod)

	// 获取已选节点
	a.mu.RLock()
	state, exists := a.deploymentStates[deploymentName]
	selectedNodes := []string{}
	if exists {
		selectedNodes = state.SelectedNodes
	}
	a.mu.RUnlock()

	// 重置环境
	envState := a.env.Reset(metrics)
	if envState == nil {
		return nil, fmt.Errorf("failed to reset environment")
	}

	// 标记已选节点
	for _, nodeName := range selectedNodes {
		for i, node := range a.env.allNodes {
			if node.Metrics.NodeName == nodeName {
				a.env.selectedMask[i] = true
				a.env.selectedNodes = append(a.env.selectedNodes, node)
				break
			}
		}
	}

	// 更新环境状态
	envState = a.env.getState()
	a.env.currentCoverage = a.env.calculateCoverageRatio()

	// 使用策略网络获取每个节点的选择概率
	probs := a.policy.Forward(envState)

	// 转换为分数
	scores := make([]algorithm.NodeScore, len(metrics))
	for i, m := range metrics {
		prob := 0.0
		if i < len(probs) {
			prob = probs[i]
		}

		// 如果已选择，分数为 0
		if a.env.selectedMask[i] {
			scores[i] = algorithm.NodeScore{
				NodeName: m.NodeName,
				Score:    0,
				Reason:   fmt.Sprintf("already selected (coverage: %.2f%%)", a.env.currentCoverage*100),
			}
		} else {
			// 将概率转换为分数 (0-100)
			scores[i] = algorithm.NodeScore{
				NodeName: m.NodeName,
				Score:    prob * 100,
				Reason:   fmt.Sprintf("rl_prob=%.4f, coverage=%.2f%%", prob, a.env.currentCoverage*100),
			}
		}
	}

	return scores, nil
}

// RecordBinding 记录 Pod 绑定
func (a *RLCoverageAlgorithm) RecordBinding(pod *v1.Pod, nodeName string) {
	deploymentName := getDeploymentName(pod)

	a.mu.Lock()
	defer a.mu.Unlock()

	state, exists := a.deploymentStates[deploymentName]
	if !exists {
		state = &DeploymentState{
			SelectedNodes: []string{},
		}
		a.deploymentStates[deploymentName] = state
	}

	// 添加节点
	if !contains(state.SelectedNodes, nodeName) {
		state.SelectedNodes = append(state.SelectedNodes, nodeName)
	}
}

// LockDeployment 锁定 Deployment
func (a *RLCoverageAlgorithm) LockDeployment(deploymentName string) {
	a.locksMu.Lock()
	lock, exists := a.deploymentLocks[deploymentName]
	if !exists {
		lock = &sync.Mutex{}
		a.deploymentLocks[deploymentName] = lock
	}
	a.locksMu.Unlock()

	lock.Lock()
}

// UnlockDeployment 解锁 Deployment
func (a *RLCoverageAlgorithm) UnlockDeployment(deploymentName string) {
	a.locksMu.Lock()
	lock, exists := a.deploymentLocks[deploymentName]
	a.locksMu.Unlock()

	if exists {
		lock.Unlock()
	}
}

// SelectNodes 使用 RL 选择节点 (批量选择，用于初始部署)
func (a *RLCoverageAlgorithm) SelectNodes(metrics []*models.UAVMetrics) ([]string, float64, error) {
	if len(metrics) == 0 {
		return nil, 0, fmt.Errorf("no metrics available")
	}

	// 重置环境
	state := a.env.Reset(metrics)
	if state == nil {
		return nil, 0, fmt.Errorf("failed to reset environment")
	}

	// 使用策略网络选择节点
	selectedIndices := []int{}
	maxSteps := len(metrics)

	for step := 0; step < maxSteps; step++ {
		// 【关键优化】一旦达到目标覆盖率，立即停止
		if a.env.GetCurrentCoverage() >= a.config.TargetCoverage {
			break
		}

		// 选择动作 (不探索，使用最优策略)
		action := a.policy.SelectAction(state, false)

		if action.NodeIndex < 0 || action.NodeIndex >= len(a.env.allNodes) {
			break
		}

		// 执行动作，更新状态
		nextState, _, done := a.env.Step(action)
		state = nextState // 关键：更新状态用于下一次决策

		selectedIndices = append(selectedIndices, action.NodeIndex)

		if done {
			break
		}
	}

	// 【节点效率优化】尝试移除贡献最小的节点
	selectedIndices = a.optimizeNodeSelection(selectedIndices)

	// 转换为节点名称
	selectedNodes := make([]string, len(selectedIndices))
	for i, idx := range selectedIndices {
		selectedNodes[i] = a.env.allNodes[idx].Metrics.NodeName
	}

	return selectedNodes, a.env.GetCurrentCoverage(), nil
}

// optimizeNodeSelection 优化节点选择，移除冗余节点
func (a *RLCoverageAlgorithm) optimizeNodeSelection(indices []int) []int {
	if len(indices) <= 1 {
		return indices
	}

	targetCov := a.config.TargetCoverage

	// 尝试逐个移除节点，看是否仍能满足覆盖率
	optimized := make([]int, len(indices))
	copy(optimized, indices)

	improved := true
	for improved {
		improved = false

		// 找到移除后覆盖率下降最小的节点
		bestRemoveIdx := -1
		bestCovAfterRemove := 0.0

		for i := 0; i < len(optimized); i++ {
			// 计算移除这个节点后的覆盖率
			remaining := make([]int, 0, len(optimized)-1)
			for j, idx := range optimized {
				if j != i {
					remaining = append(remaining, idx)
				}
			}

			// 重新计算覆盖率
			covAfter := a.calculateCoverageForIndices(remaining)

			// 如果移除后仍满足目标，且是目前最好的选择
			if covAfter >= targetCov && covAfter > bestCovAfterRemove {
				bestRemoveIdx = i
				bestCovAfterRemove = covAfter
			}
		}

		// 如果找到可以移除的节点
		if bestRemoveIdx >= 0 {
			newOptimized := make([]int, 0, len(optimized)-1)
			for i, idx := range optimized {
				if i != bestRemoveIdx {
					newOptimized = append(newOptimized, idx)
				}
			}
			optimized = newOptimized
			improved = true

			// 更新环境状态
			a.env.selectedNodes = make([]*NodeInfo, len(optimized))
			a.env.selectedMask = make([]bool, len(a.env.allNodes))
			for i, idx := range optimized {
				a.env.selectedNodes[i] = a.env.allNodes[idx]
				a.env.selectedMask[idx] = true
			}
			a.env.currentCoverage = bestCovAfterRemove
		}
	}

	return optimized
}

// calculateCoverageForIndices 计算指定索引节点的覆盖率
func (a *RLCoverageAlgorithm) calculateCoverageForIndices(indices []int) float64 {
	if len(indices) == 0 {
		return 0
	}

	nodes := make([]*NodeInfo, len(indices))
	for i, idx := range indices {
		nodes[i] = a.env.allNodes[idx]
	}

	coverage := a.env.calculateCoverage(nodes)
	return coverage / a.env.maxCoverage
}

// Train 训练模型
func (a *RLCoverageAlgorithm) Train(trainingData [][]*models.UAVMetrics, numEpisodes int) TrainingStats {
	stats := TrainingStats{}

	for episode := 0; episode < numEpisodes; episode++ {
		// 随机选择训练数据
		dataIdx := episode % len(trainingData)
		metrics := trainingData[dataIdx]

		// 收集一个 episode 的经验
		episodes := []Episode{}
		ep := a.collectEpisode(metrics)
		episodes = append(episodes, ep)

		stats.AvgReward += ep.TotalReward
		stats.AvgCoverage += ep.FinalCoverage
		stats.AvgNodes += float64(ep.SelectedNodes)

		// 每隔一定 episode 更新网络
		if (episode+1)%a.config.EpisodesPerUpdate == 0 {
			gradients := a.policy.GetGradients(episodes)
			a.policy.UpdateWeights(gradients, a.config.LearningRate)
			episodes = []Episode{}
		}
	}

	// 计算平均值
	stats.Episode = numEpisodes
	stats.AvgReward /= float64(numEpisodes)
	stats.AvgCoverage /= float64(numEpisodes)
	stats.AvgNodes /= float64(numEpisodes)

	a.isTrained = true
	return stats
}

// collectEpisode 收集一个 episode 的经验
func (a *RLCoverageAlgorithm) collectEpisode(metrics []*models.UAVMetrics) Episode {
	ep := Episode{
		Experiences: []Experience{},
	}

	state := a.env.Reset(metrics)
	if state == nil {
		return ep
	}

	totalReward := 0.0
	maxSteps := a.config.MaxStepsPerEpisode

	for step := 0; step < maxSteps; step++ {
		// 选择动作 (探索)
		action := a.policy.SelectAction(state, true)

		if action.NodeIndex < 0 {
			break
		}

		// 执行动作
		nextState, reward, done := a.env.Step(action)

		// 记录经验
		ep.Experiences = append(ep.Experiences, Experience{
			State:     state,
			Action:    action,
			Reward:    reward,
			NextState: nextState,
			Done:      done,
		})

		totalReward += reward
		state = nextState

		if done {
			break
		}
	}

	ep.TotalReward = totalReward
	ep.FinalCoverage = a.env.GetCurrentCoverage()
	ep.SelectedNodes = len(a.env.selectedNodes)

	return ep
}

// SaveModel 保存模型
func (a *RLCoverageAlgorithm) SaveModel(filepath string) error {
	return a.policy.Save(filepath)
}

// LoadModel 加载模型
func (a *RLCoverageAlgorithm) LoadModel(filepath string) error {
	err := a.policy.Load(filepath)
	if err == nil {
		a.isTrained = true
	}
	return err
}

// IsTrained 是否已训练
func (a *RLCoverageAlgorithm) IsTrained() bool {
	return a.isTrained
}

// GetConfig 获取配置
func (a *RLCoverageAlgorithm) GetConfig() *RLConfig {
	return a.config
}

// CleanupDeployment 清理 Deployment 状态
func (a *RLCoverageAlgorithm) CleanupDeployment(deploymentName string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.deploymentStates, deploymentName)
}

// 辅助函数

func getDeploymentName(pod *v1.Pod) string {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			return owner.Name
		}
	}
	return pod.Name
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

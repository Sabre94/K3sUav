package greed_nsgaii

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
	v1 "k8s.io/api/core/v1"
)

// NodeScore 节点评分结果（避免循环导入，本地定义）
type NodeScore struct {
	NodeName string  // 节点名称
	Score    float64 // 分数 (0-100)
	Reason   string  // 评分原因
}

// GreedNSGAIIAlgorithm GREED + NSGA-II 混合调度算法
// 两阶段算法：
// 1. Greedy Phase: 贪心选择满足覆盖率约束的节点子集
// 2. NSGA-II Phase: 多目标优化（电量、延迟、利用率、节点数）
type GreedNSGAIIAlgorithm struct {
	// 配置参数
	taskType       TaskType
	coverageConfig *CoverageConfig
	nsga2Config    *NSGA2Config

	// 状态缓存：记录每个 Deployment 已选择的节点
	deploymentStates map[string]*DeploymentState
	mu               sync.RWMutex

	// 每个 Deployment 的调度锁（确保串行调度）
	deploymentLocks map[string]*sync.Mutex
	locksmu         sync.Mutex

	// GPS 转换器（使用第一个节点作为基准点）
	gpsConverter *GPSConverter
	converterMu  sync.Once
}

// NewGreedNSGAIIAlgorithm 创建 GREED + NSGA-II 算法实例
func NewGreedNSGAIIAlgorithm(
	taskType TaskType,
	targetCoverageRatio float64,
	coverageRadius float64,
) *GreedNSGAIIAlgorithm {
	coverageConfig := &CoverageConfig{
		TargetCoverageRatio: targetCoverageRatio,
		CoverageRadius:      coverageRadius,
		GridDensity:         50, // 默认网格密度
	}

	nsga2Config := &NSGA2Config{
		PopulationSize:   50,
		Generations:      30,
		CrossoverRate:    0.9,
		MutationRate:     0.1,
		CoverageConfig:   coverageConfig,
		GreedySelector:   NewGreedySelector(coverageConfig),
	}

	return &GreedNSGAIIAlgorithm{
		taskType:         taskType,
		coverageConfig:   coverageConfig,
		nsga2Config:      nsga2Config,
		deploymentStates: make(map[string]*DeploymentState),
		deploymentLocks:  make(map[string]*sync.Mutex),
	}
}

// Name 返回算法名称
func (a *GreedNSGAIIAlgorithm) Name() string {
	return "greed-nsgaii"
}

// Filter 过滤节点（不做硬性过滤）
func (a *GreedNSGAIIAlgorithm) Filter(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]*models.UAVMetrics, error) {
	return metrics, nil
}

// Score 为每个节点计算分数
func (a *GreedNSGAIIAlgorithm) Score(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]NodeScore, error) {
	if len(metrics) == 0 {
		return nil, fmt.Errorf("no metrics available")
	}

	// 1. 初始化 GPS 转换器（使用第一个节点作为基准点）
	a.converterMu.Do(func() {
		a.gpsConverter = NewGPSConverter(metrics[0].GPS.Latitude, metrics[0].GPS.Longitude)
	})

	// 2. 计算所有节点的得分和 XY 坐标
	scorer := NewNodeScorer(a.taskType)
	allNodes := a.convertMetricsToNodeInfo(metrics, scorer)

	// 3. 获取 Pod 所属的 Deployment
	deploymentName := getDeploymentName(pod)

	// 4. 获取该 Deployment 已选择的节点（只读）
	a.mu.RLock()
	state, exists := a.deploymentStates[deploymentName]
	if !exists {
		state = &DeploymentState{
			SelectedNodes:     []string{},
			SelectedNodesInfo: []*NodeInfo{},
		}
	}

	// 复制已选节点信息（避免并发修改）
	selectedNodes := make([]*NodeInfo, len(state.SelectedNodesInfo))
	copy(selectedNodes, state.SelectedNodesInfo)
	selectedNodeNames := make([]string, len(state.SelectedNodes))
	copy(selectedNodeNames, state.SelectedNodes)
	maxPossibleArea := state.MaxPossibleArea
	a.mu.RUnlock()

	// 5. 如果是第一个 Pod，计算最大可能覆盖面积
	if maxPossibleArea == 0 {
		maxPossibleArea = CalculateMaxPossibleArea(allNodes, a.coverageConfig.CoverageRadius, a.coverageConfig.GridDensity)
	}

	// 6. 使用 Greedy 算法为每个节点计算增量得分
	greedy := NewGreedySelector(a.coverageConfig)
	scores := []NodeScore{}

	for _, node := range allNodes {
		// 跳过已选择的节点
		if IsNodeAlreadySelected(node.Metrics.NodeName, selectedNodes) {
			scores = append(scores, NodeScore{
				NodeName: node.Metrics.NodeName,
				Score:    0.0,
				Reason:   fmt.Sprintf("already selected (%d/%d nodes)", len(selectedNodes), len(allNodes)),
			})
			continue
		}

		// 计算增量得分
		incrementalScore := greedy.ScoreNodeForIncremental(node, selectedNodes, maxPossibleArea)

		scores = append(scores, NodeScore{
			NodeName: node.Metrics.NodeName,
			Score:    incrementalScore * 100, // 缩放到 0-100
			Reason: fmt.Sprintf("incremental_score=%.2f, node_score=%.2f, task=%s",
				incrementalScore, node.Score, a.taskType),
		})
	}

	return scores, nil
}

// RecordBinding 记录 Pod 绑定到节点（贪心算法的关键：绑定后才更新缓存）
func (a *GreedNSGAIIAlgorithm) RecordBinding(pod *v1.Pod, nodeName string, allMetrics []*models.UAVMetrics) {
	deploymentName := getDeploymentName(pod)

	a.mu.Lock()
	defer a.mu.Unlock()

	state, exists := a.deploymentStates[deploymentName]
	if !exists {
		state = &DeploymentState{
			SelectedNodes:     []string{},
			SelectedNodesInfo: []*NodeInfo{},
		}
		a.deploymentStates[deploymentName] = state
	}

	// 检查节点是否已存在
	if contains(state.SelectedNodes, nodeName) {
		return
	}

	// 找到该节点的 NodeInfo
	scorer := NewNodeScorer(a.taskType)
	allNodes := a.convertMetricsToNodeInfo(allMetrics, scorer)

	var selectedNode *NodeInfo
	for _, node := range allNodes {
		if node.Metrics.NodeName == nodeName {
			selectedNode = node
			break
		}
	}

	if selectedNode == nil {
		return
	}

	// 更新状态
	state.SelectedNodes = append(state.SelectedNodes, nodeName)
	state.SelectedNodesInfo = append(state.SelectedNodesInfo, selectedNode)

	// 更新覆盖面积
	plotArea := CalculatePlotArea(state.SelectedNodesInfo, a.coverageConfig.CoverageRadius)
	state.CurrentCoverageArea = CalculateUnionArea(state.SelectedNodesInfo, plotArea, a.coverageConfig.CoverageRadius, a.coverageConfig.GridDensity)

	// 计算最大可能覆盖面积（第一次）
	if state.MaxPossibleArea == 0 {
		state.MaxPossibleArea = CalculateMaxPossibleArea(allNodes, a.coverageConfig.CoverageRadius, a.coverageConfig.GridDensity)
	}
}

// RunNSGA2Optimization 运行 NSGA-II 优化（可选，用于离线优化或实验）
func (a *GreedNSGAIIAlgorithm) RunNSGA2Optimization(allMetrics []*models.UAVMetrics) *NSGA2Result {
	// 转换节点信息
	scorer := NewNodeScorer(a.taskType)
	allNodes := a.convertMetricsToNodeInfo(allMetrics, scorer)

	// 创建 NSGA-II 优化器
	optimizer := NewNSGA2Optimizer(a.nsga2Config, allNodes)

	// 执行优化
	return optimizer.Optimize()
}

// convertMetricsToNodeInfo 将 UAVMetrics 转换为 NodeInfo
func (a *GreedNSGAIIAlgorithm) convertMetricsToNodeInfo(metrics []*models.UAVMetrics, scorer *NodeScorer) []*NodeInfo {
	nodeInfos := make([]*NodeInfo, len(metrics))

	for i, m := range metrics {
		x, y := a.gpsConverter.GPSToXY(m.GPS.Latitude, m.GPS.Longitude)
		nodeInfos[i] = &NodeInfo{
			Metrics: m,
			Score:   scorer.CalculateScore(m),
			XMeters: x,
			YMeters: y,
		}
	}

	return nodeInfos
}

// LockDeployment 锁定 Deployment 的调度（确保串行调度）
func (a *GreedNSGAIIAlgorithm) LockDeployment(deploymentName string) {
	a.locksmu.Lock()
	lock, exists := a.deploymentLocks[deploymentName]
	if !exists {
		lock = &sync.Mutex{}
		a.deploymentLocks[deploymentName] = lock
	}
	a.locksmu.Unlock()

	lock.Lock()
}

// UnlockDeployment 解锁 Deployment 的调度
func (a *GreedNSGAIIAlgorithm) UnlockDeployment(deploymentName string) {
	a.locksmu.Lock()
	lock, exists := a.deploymentLocks[deploymentName]
	a.locksmu.Unlock()

	if exists {
		lock.Unlock()
	}
}

// CleanupDeployment 清理指定 Deployment 的状态缓存
func (a *GreedNSGAIIAlgorithm) CleanupDeployment(deploymentName string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.deploymentStates, deploymentName)
}

// GetDeploymentState 获取指定 Deployment 的状态（用于调试）
func (a *GreedNSGAIIAlgorithm) GetDeploymentState(deploymentName string) *DeploymentState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.deploymentStates[deploymentName]
}

// getDeploymentName 从 Pod 的 OwnerReferences 中获取 Deployment 名称
func getDeploymentName(pod *v1.Pod) string {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			return owner.Name
		}
	}
	return pod.Name
}

// contains 检查切片中是否包含某个元素
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GetCoverageInfo 获取当前覆盖率信息
func (a *GreedNSGAIIAlgorithm) GetCoverageInfo(deploymentName string) (currentCoverage float64, maxArea float64, numNodes int) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	state, exists := a.deploymentStates[deploymentName]
	if !exists {
		return 0, 0, 0
	}

	coverageRatio := 0.0
	if state.MaxPossibleArea > 0 {
		coverageRatio = state.CurrentCoverageArea / state.MaxPossibleArea
	}

	return coverageRatio, state.MaxPossibleArea, len(state.SelectedNodes)
}

// GetDeploymentCoverageInfo 获取 Deployment 的详细覆盖信息
type DeploymentCoverageInfo struct {
	DeploymentName      string    `json:"deployment_name"`
	SelectedNodes       []string  `json:"selected_nodes"`
	NumNodes            int       `json:"num_nodes"`
	CurrentCoverageArea float64   `json:"current_coverage_area"`
	MaxPossibleArea     float64   `json:"max_possible_area"`
	CoverageRatio       float64   `json:"coverage_ratio"`
	LastUpdate          time.Time `json:"last_update"`
}

// GetAllDeploymentCoverageInfo 获取所有 Deployment 的覆盖信息
func (a *GreedNSGAIIAlgorithm) GetAllDeploymentCoverageInfo() []DeploymentCoverageInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	infos := make([]DeploymentCoverageInfo, 0, len(a.deploymentStates))

	for deploymentName, state := range a.deploymentStates {
		coverageRatio := 0.0
		if state.MaxPossibleArea > 0 {
			coverageRatio = state.CurrentCoverageArea / state.MaxPossibleArea
		}

		infos = append(infos, DeploymentCoverageInfo{
			DeploymentName:      deploymentName,
			SelectedNodes:       state.SelectedNodes,
			NumNodes:            len(state.SelectedNodes),
			CurrentCoverageArea: state.CurrentCoverageArea,
			MaxPossibleArea:     state.MaxPossibleArea,
			CoverageRatio:       coverageRatio,
			LastUpdate:          time.Now(),
		})
	}

	return infos
}

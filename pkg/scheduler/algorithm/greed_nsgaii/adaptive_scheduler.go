package greed_nsgaii

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// AdaptiveAction 自适应调度动作
type AdaptiveAction string

const (
	ActionNone      AdaptiveAction = "none"       // 无需操作
	ActionGreedy    AdaptiveAction = "greedy"     // 贪心补充
	ActionReplan    AdaptiveAction = "replan"     // NSGA-II 重规划
)

// AdaptiveConfig 自适应调度配置
type AdaptiveConfig struct {
	// 覆盖率阈值
	TargetCoverageRatio   float64 // 目标覆盖率 (e.g., 0.9 = 90%)
	MinCoverageRatio      float64 // 最低可接受覆盖率 (e.g., 0.7 = 70%)

	// 变化阈值
	MinorDropThreshold    float64 // 小幅下降阈值 (e.g., 0.1 = 10%)
	MajorDropThreshold    float64 // 大幅下降阈值 (e.g., 0.3 = 30%)

	// 监控配置
	MonitorInterval       time.Duration // 监控间隔
	NodeTimeoutDuration   time.Duration // 节点超时时间（认为离线）

	// 算法配置
	CoverageRadius        float64 // 覆盖半径（米）
	GridDensity           int     // 网格密度
}

// DefaultAdaptiveConfig 默认配置
func DefaultAdaptiveConfig() *AdaptiveConfig {
	return &AdaptiveConfig{
		TargetCoverageRatio:   0.90,
		MinCoverageRatio:      0.70,
		MinorDropThreshold:    0.10,
		MajorDropThreshold:    0.30,
		MonitorInterval:       30 * time.Second,
		NodeTimeoutDuration:   60 * time.Second,
		CoverageRadius:        500.0,
		GridDensity:           50,
	}
}

// CoverageState 覆盖率状态
type CoverageState struct {
	DeploymentName      string
	CurrentCoverage     float64   // 当前覆盖率 (0-1)
	TargetCoverage      float64   // 目标覆盖率
	LastCoverage        float64   // 上次覆盖率
	CoverageDropRate    float64   // 覆盖率下降幅度 (0-1)
	ActiveNodes         []string  // 活跃节点
	OfflineNodes        []string  // 离线节点
	TotalNodes          int       // 总节点数
	LastUpdate          time.Time
	RecommendedAction   AdaptiveAction
}

// AdaptiveScheduler 自适应调度器
type AdaptiveScheduler struct {
	config       *AdaptiveConfig
	taskType     TaskType

	// 状态缓存
	stateCache   map[string]*CoverageState  // deploymentName -> state
	mu           sync.RWMutex

	// GPS 转换器
	gpsConverter *GPSConverter
	converterMu  sync.Once

	// 监控控制
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewAdaptiveScheduler 创建自适应调度器
func NewAdaptiveScheduler(config *AdaptiveConfig, taskType TaskType) *AdaptiveScheduler {
	if config == nil {
		config = DefaultAdaptiveConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &AdaptiveScheduler{
		config:     config,
		taskType:   taskType,
		stateCache: make(map[string]*CoverageState),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// InitializeDeployment 初始化 Deployment 的覆盖率跟踪
func (s *AdaptiveScheduler) InitializeDeployment(deploymentName string, selectedNodes []string, allMetrics []*models.UAVMetrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 初始化 GPS 转换器
	if len(allMetrics) > 0 {
		s.converterMu.Do(func() {
			s.gpsConverter = NewGPSConverter(allMetrics[0].GPS.Latitude, allMetrics[0].GPS.Longitude)
		})
	}

	// 计算初始覆盖率
	coverage := s.calculateCoverage(selectedNodes, allMetrics)

	s.stateCache[deploymentName] = &CoverageState{
		DeploymentName:    deploymentName,
		CurrentCoverage:   coverage,
		TargetCoverage:    s.config.TargetCoverageRatio,
		LastCoverage:      coverage,
		CoverageDropRate:  0,
		ActiveNodes:       selectedNodes,
		OfflineNodes:      []string{},
		TotalNodes:        len(allMetrics),
		LastUpdate:        time.Now(),
		RecommendedAction: ActionNone,
	}

	return nil
}

// CheckAndDecide 检查覆盖率并决定动作
// 这是核心方法：根据覆盖率变化决定使用哪种策略
func (s *AdaptiveScheduler) CheckAndDecide(deploymentName string, allMetrics []*models.UAVMetrics) (*CoverageState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.stateCache[deploymentName]
	if !exists {
		return nil, fmt.Errorf("deployment %s not found in cache", deploymentName)
	}

	// 1. 检测离线节点
	activeNodes, offlineNodes := s.detectOfflineNodes(state.ActiveNodes, allMetrics)

	// 2. 计算当前覆盖率（仅使用活跃节点）
	currentCoverage := s.calculateCoverage(activeNodes, allMetrics)

	// 3. 计算覆盖率下降幅度
	dropRate := 0.0
	if state.LastCoverage > 0 {
		dropRate = (state.LastCoverage - currentCoverage) / state.LastCoverage
	}

	// 4. 决定动作
	action := s.decideAction(currentCoverage, dropRate)

	// 5. 更新状态
	state.LastCoverage = state.CurrentCoverage
	state.CurrentCoverage = currentCoverage
	state.CoverageDropRate = dropRate
	state.ActiveNodes = activeNodes
	state.OfflineNodes = offlineNodes
	state.LastUpdate = time.Now()
	state.RecommendedAction = action

	return state, nil
}

// decideAction 根据覆盖率变化决定动作
func (s *AdaptiveScheduler) decideAction(currentCoverage, dropRate float64) AdaptiveAction {
	// 情况1: 覆盖率低于最低可接受阈值 -> 强制重规划
	if currentCoverage < s.config.MinCoverageRatio {
		return ActionReplan
	}

	// 情况2: 大幅下降 (>30%) -> 重规划
	if dropRate >= s.config.MajorDropThreshold {
		return ActionReplan
	}

	// 情况3: 中等下降 (10-30%) -> 贪心补充
	if dropRate >= s.config.MinorDropThreshold {
		return ActionGreedy
	}

	// 情况4: 覆盖率低于目标但未大幅下降 -> 贪心补充
	if currentCoverage < s.config.TargetCoverageRatio {
		return ActionGreedy
	}

	// 情况5: 一切正常
	return ActionNone
}

// detectOfflineNodes 检测离线节点
func (s *AdaptiveScheduler) detectOfflineNodes(selectedNodes []string, allMetrics []*models.UAVMetrics) (active, offline []string) {
	// 构建节点最后更新时间映射
	nodeLastUpdate := make(map[string]time.Time)
	for _, m := range allMetrics {
		nodeLastUpdate[m.NodeName] = m.GPS.LastUpdate
	}

	now := time.Now()
	for _, nodeName := range selectedNodes {
		lastUpdate, exists := nodeLastUpdate[nodeName]
		if !exists {
			// 节点不在 metrics 中，认为离线
			offline = append(offline, nodeName)
			continue
		}

		if now.Sub(lastUpdate) > s.config.NodeTimeoutDuration {
			// 超时，认为离线
			offline = append(offline, nodeName)
		} else {
			active = append(active, nodeName)
		}
	}

	return active, offline
}

// calculateCoverage 计算覆盖率
func (s *AdaptiveScheduler) calculateCoverage(selectedNodes []string, allMetrics []*models.UAVMetrics) float64 {
	if len(selectedNodes) == 0 || len(allMetrics) == 0 {
		return 0.0
	}

	// 转换所有节点为 NodeInfo
	scorer := NewNodeScorer(s.taskType)
	allNodeInfos := s.convertMetricsToNodeInfo(allMetrics, scorer)

	// 筛选出选中的节点
	selectedNodeInfos := []*NodeInfo{}
	for _, nodeInfo := range allNodeInfos {
		for _, name := range selectedNodes {
			if nodeInfo.Metrics.NodeName == name {
				selectedNodeInfos = append(selectedNodeInfos, nodeInfo)
				break
			}
		}
	}

	if len(selectedNodeInfos) == 0 {
		return 0.0
	}

	// 计算最大可能覆盖面积
	maxArea := CalculateMaxPossibleArea(allNodeInfos, s.config.CoverageRadius, s.config.GridDensity)
	if maxArea == 0 {
		return 0.0
	}

	// 计算当前覆盖面积
	plotArea := CalculatePlotArea(selectedNodeInfos, s.config.CoverageRadius)
	currentArea := CalculateUnionArea(selectedNodeInfos, plotArea, s.config.CoverageRadius, s.config.GridDensity)

	return currentArea / maxArea
}

// convertMetricsToNodeInfo 转换 metrics 为 NodeInfo
func (s *AdaptiveScheduler) convertMetricsToNodeInfo(metrics []*models.UAVMetrics, scorer *NodeScorer) []*NodeInfo {
	if s.gpsConverter == nil && len(metrics) > 0 {
		s.gpsConverter = NewGPSConverter(metrics[0].GPS.Latitude, metrics[0].GPS.Longitude)
	}

	nodeInfos := make([]*NodeInfo, len(metrics))
	for i, m := range metrics {
		x, y := s.gpsConverter.GPSToXY(m.GPS.Latitude, m.GPS.Longitude)
		nodeInfos[i] = &NodeInfo{
			Metrics: m,
			Score:   scorer.CalculateScore(m),
			XMeters: x,
			YMeters: y,
		}
	}
	return nodeInfos
}

// ExecuteGreedyRepair 执行贪心修复（补充节点）
func (s *AdaptiveScheduler) ExecuteGreedyRepair(deploymentName string, allMetrics []*models.UAVMetrics) ([]string, error) {
	s.mu.Lock()
	state, exists := s.stateCache[deploymentName]
	if !exists {
		s.mu.Unlock()
		return nil, fmt.Errorf("deployment %s not found", deploymentName)
	}

	currentNodes := make([]string, len(state.ActiveNodes))
	copy(currentNodes, state.ActiveNodes)
	s.mu.Unlock()

	// 转换为 NodeInfo
	scorer := NewNodeScorer(s.taskType)
	allNodeInfos := s.convertMetricsToNodeInfo(allMetrics, scorer)

	// 获取当前已选节点的 NodeInfo
	selectedNodeInfos := FilterNodesByNames(allNodeInfos, currentNodes)

	// 计算需要补充的节点
	coverageConfig := &CoverageConfig{
		TargetCoverageRatio: s.config.TargetCoverageRatio,
		CoverageRadius:      s.config.CoverageRadius,
		GridDensity:         s.config.GridDensity,
	}

	greedy := NewGreedySelector(coverageConfig)

	// 计算最大可能覆盖面积
	maxArea := CalculateMaxPossibleArea(allNodeInfos, s.config.CoverageRadius, s.config.GridDensity)
	targetArea := maxArea * s.config.TargetCoverageRatio

	// 计算当前覆盖面积
	plotArea := CalculatePlotArea(allNodeInfos, s.config.CoverageRadius)
	currentArea := CalculateUnionArea(selectedNodeInfos, plotArea, s.config.CoverageRadius, s.config.GridDensity)

	// 贪心补充节点直到达到目标覆盖率
	newNodes := []string{}
	for currentArea < targetArea {
		// 找出最佳补充节点
		bestNode := s.findBestNodeToAdd(selectedNodeInfos, allNodeInfos, greedy, maxArea)
		if bestNode == nil {
			break
		}

		// 添加节点
		selectedNodeInfos = append(selectedNodeInfos, bestNode)
		newNodes = append(newNodes, bestNode.Metrics.NodeName)

		// 更新覆盖面积
		plotArea = CalculatePlotArea(selectedNodeInfos, s.config.CoverageRadius)
		currentArea = CalculateUnionArea(selectedNodeInfos, plotArea, s.config.CoverageRadius, s.config.GridDensity)
	}

	// 更新缓存
	s.mu.Lock()
	state.ActiveNodes = append(state.ActiveNodes, newNodes...)
	state.CurrentCoverage = currentArea / maxArea
	state.LastUpdate = time.Now()
	s.mu.Unlock()

	return newNodes, nil
}

// findBestNodeToAdd 找出最佳补充节点
func (s *AdaptiveScheduler) findBestNodeToAdd(selectedNodes, allNodes []*NodeInfo, greedy *GreedySelector, maxArea float64) *NodeInfo {
	var bestNode *NodeInfo
	bestScore := -1.0

	for _, node := range allNodes {
		// 跳过已选节点
		if IsNodeAlreadySelected(node.Metrics.NodeName, selectedNodes) {
			continue
		}

		// 计算增量得分
		score := greedy.ScoreNodeForIncremental(node, selectedNodes, maxArea)
		if score > bestScore {
			bestScore = score
			bestNode = node
		}
	}

	return bestNode
}

// ExecuteNSGA2Replan 执行 NSGA-II 重规划
func (s *AdaptiveScheduler) ExecuteNSGA2Replan(deploymentName string, allMetrics []*models.UAVMetrics) ([]string, *NSGA2Result, error) {
	// 创建 NSGA-II 算法实例
	algo := NewGreedNSGAIIAlgorithm(
		s.taskType,
		s.config.TargetCoverageRatio,
		s.config.CoverageRadius,
	)

	// 初始化 GPS 转换器（通过调用 Score）
	s.mu.RLock()
	if s.gpsConverter != nil {
		algo.gpsConverter = s.gpsConverter
	}
	s.mu.RUnlock()

	// 如果还没初始化，手动初始化
	if algo.gpsConverter == nil && len(allMetrics) > 0 {
		algo.gpsConverter = NewGPSConverter(allMetrics[0].GPS.Latitude, allMetrics[0].GPS.Longitude)
	}

	// 执行 NSGA-II 优化
	result := algo.RunNSGA2Optimization(allMetrics)

	if result.BestSolution == nil {
		return nil, result, fmt.Errorf("NSGA-II failed to find a solution")
	}

	// 提取选中的节点
	selectedNodes := []string{}
	for i, selected := range result.BestSolution.Chromosome {
		if selected && i < len(allMetrics) {
			selectedNodes = append(selectedNodes, allMetrics[i].NodeName)
		}
	}

	// 更新缓存
	s.mu.Lock()
	if state, exists := s.stateCache[deploymentName]; exists {
		state.ActiveNodes = selectedNodes
		state.OfflineNodes = []string{}
		state.CurrentCoverage = -result.BestSolution.Objectives[0] / 100 // 取负值还原
		state.LastUpdate = time.Now()
	}
	s.mu.Unlock()

	return selectedNodes, result, nil
}

// StartMonitoring 启动持续监控
func (s *AdaptiveScheduler) StartMonitoring(deploymentName string, metricsProvider func() []*models.UAVMetrics, callback func(*CoverageState)) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(s.config.MonitorInterval)
		defer ticker.Stop()

		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				// 获取最新 metrics
				metrics := metricsProvider()
				if len(metrics) == 0 {
					continue
				}

				// 检查并决定动作
				state, err := s.CheckAndDecide(deploymentName, metrics)
				if err != nil {
					continue
				}

				// 回调通知
				if callback != nil {
					callback(state)
				}
			}
		}
	}()
}

// StopMonitoring 停止监控
func (s *AdaptiveScheduler) StopMonitoring() {
	s.cancel()
	s.wg.Wait()
}

// GetState 获取当前状态
func (s *AdaptiveScheduler) GetState(deploymentName string) *CoverageState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if state, exists := s.stateCache[deploymentName]; exists {
		// 返回副本
		stateCopy := *state
		stateCopy.ActiveNodes = make([]string, len(state.ActiveNodes))
		copy(stateCopy.ActiveNodes, state.ActiveNodes)
		stateCopy.OfflineNodes = make([]string, len(state.OfflineNodes))
		copy(stateCopy.OfflineNodes, state.OfflineNodes)
		return &stateCopy
	}
	return nil
}

// RemoveNode 手动移除节点（节点失效时调用）
func (s *AdaptiveScheduler) RemoveNode(deploymentName, nodeName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.stateCache[deploymentName]
	if !exists {
		return
	}

	// 从活跃节点中移除
	newActive := []string{}
	for _, n := range state.ActiveNodes {
		if n != nodeName {
			newActive = append(newActive, n)
		}
	}
	state.ActiveNodes = newActive

	// 添加到离线节点
	state.OfflineNodes = append(state.OfflineNodes, nodeName)
}

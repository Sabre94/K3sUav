package greed_nsgaii

import (
	"fmt"
)

// GreedySelector 贪心节点选择器
type GreedySelector struct {
	config *CoverageConfig
}

// NewGreedySelector 创建贪心选择器
func NewGreedySelector(config *CoverageConfig) *GreedySelector {
	return &GreedySelector{
		config: config,
	}
}

// SelectNodes 贪心选择节点以满足覆盖率要求
// 核心思想：每次选择 (增量覆盖面积 × 节点得分) 最大的节点
func (g *GreedySelector) SelectNodes(allNodes []*NodeInfo) (*GreedyResult, error) {
	if len(allNodes) == 0 {
		return nil, fmt.Errorf("no nodes available")
	}

	// 1. 计算绘图区域和最大可能覆盖面积
	plotArea := CalculatePlotArea(allNodes, g.config.CoverageRadius)
	maxPossibleArea := CalculateUnionArea(allNodes, plotArea, g.config.CoverageRadius, g.config.GridDensity)

	if maxPossibleArea == 0 {
		return nil, fmt.Errorf("max possible area is zero")
	}

	// 2. 初始化选中节点列表和剩余节点列表
	selectedNodes := []*NodeInfo{}
	remainingNodes := make([]*NodeInfo, len(allNodes))
	copy(remainingNodes, allNodes)

	currentArea := 0.0
	targetArea := maxPossibleArea * g.config.TargetCoverageRatio

	// 3. 贪心循环：每次选择增益最大的节点
	for len(remainingNodes) > 0 {
		// 检查是否已满足覆盖率要求
		if currentArea >= targetArea {
			break
		}

		// 找出增益最大的节点
		bestIdx := -1
		bestGain := -1.0

		for i, node := range remainingNodes {
			// 计算增量覆盖面积
			incrementalArea := CalculateIncrementalArea(node, selectedNodes, plotArea, g.config.CoverageRadius, g.config.GridDensity)

			// 增益 = 增量面积 × 节点得分
			gain := incrementalArea * node.Score

			if gain > bestGain {
				bestGain = gain
				bestIdx = i
			}
		}

		// 如果没有找到增益为正的节点，退出
		if bestIdx == -1 || bestGain <= 0 {
			break
		}

		// 选择该节点
		selectedNode := remainingNodes[bestIdx]
		selectedNodes = append(selectedNodes, selectedNode)

		// 更新当前覆盖面积
		currentArea = CalculateUnionArea(selectedNodes, plotArea, g.config.CoverageRadius, g.config.GridDensity)

		// 从剩余节点中移除
		remainingNodes = append(remainingNodes[:bestIdx], remainingNodes[bestIdx+1:]...)
	}

	// 4. 返回结果
	achievedCoverage := CalculateCoverageRatio(currentArea, maxPossibleArea)

	return &GreedyResult{
		SelectedNodes:    selectedNodes,
		FinalUnionArea:   currentArea,
		AchievedCoverage: achievedCoverage,
		MaxPossibleArea:  maxPossibleArea,
	}, nil
}

// ScoreNodeForIncremental 为单个节点计算增量得分（用于在线调度）
// 给定已选择的节点，计算新节点的增量得分
func (g *GreedySelector) ScoreNodeForIncremental(newNode *NodeInfo, selectedNodes []*NodeInfo, maxPossibleArea float64) float64 {
	if len(selectedNodes) == 0 {
		// 如果没有已选择的节点，这是第一个节点
		// 返回节点得分本身
		return newNode.Score
	}

	// 计算绘图区域
	allNodes := append(selectedNodes, newNode)
	plotArea := CalculatePlotArea(allNodes, g.config.CoverageRadius)

	// 计算增量覆盖面积
	incrementalArea := CalculateIncrementalArea(newNode, selectedNodes, plotArea, g.config.CoverageRadius, g.config.GridDensity)

	// 增量得分 = 增量面积 × 节点得分
	// 归一化到 0-100 范围
	incrementalScore := (incrementalArea / maxPossibleArea) * newNode.Score

	return incrementalScore
}

// IsNodeAlreadySelected 检查节点是否已被选择
func IsNodeAlreadySelected(nodeName string, selectedNodes []*NodeInfo) bool {
	for _, node := range selectedNodes {
		if node.Metrics.NodeName == nodeName {
			return true
		}
	}
	return false
}

// GetSelectedNodeNames 获取已选择节点的名称列表
func GetSelectedNodeNames(selectedNodes []*NodeInfo) []string {
	names := make([]string, len(selectedNodes))
	for i, node := range selectedNodes {
		names[i] = node.Metrics.NodeName
	}
	return names
}

// FilterNodesByNames 根据名称列表过滤节点
func FilterNodesByNames(allNodes []*NodeInfo, names []string) []*NodeInfo {
	filtered := make([]*NodeInfo, 0, len(names))
	nameSet := make(map[string]bool)
	for _, name := range names {
		nameSet[name] = true
	}

	for _, node := range allNodes {
		if nameSet[node.Metrics.NodeName] {
			filtered = append(filtered, node)
		}
	}

	return filtered
}

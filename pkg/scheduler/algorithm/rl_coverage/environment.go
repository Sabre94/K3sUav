package rl_coverage

import (
	"math"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// Environment RL 环境
type Environment struct {
	config *RLConfig

	// 所有节点
	allNodes []*NodeInfo

	// 当前状态
	selectedNodes []*NodeInfo
	selectedMask  []bool

	// 覆盖率计算
	plotArea      PlotArea
	gridPoints    [][2]float64
	maxCoverage   float64 // 所有节点覆盖时的面积
	currentCoverage float64

	// GPS 转换器
	gpsConverter *GPSConverter
}

// PlotArea 绘图区域
type PlotArea struct {
	XMin, XMax, YMin, YMax float64
}

// GPSConverter GPS 坐标转换器
type GPSConverter struct {
	refLat float64
	refLon float64
}

// NewGPSConverter 创建 GPS 转换器
func NewGPSConverter(refLat, refLon float64) *GPSConverter {
	return &GPSConverter{refLat: refLat, refLon: refLon}
}

// GPSToXY 将 GPS 坐标转换为平面坐标 (米)
func (g *GPSConverter) GPSToXY(lat, lon float64) (x, y float64) {
	// 简化计算: 使用等距投影
	refLatRad := g.refLat * math.Pi / 180

	// 1度纬度约等于111公里
	y = (lat - g.refLat) * 111000

	// 1度经度随纬度变化
	x = (lon - g.refLon) * 111000 * math.Cos(refLatRad)

	return x, y
}

// NewEnvironment 创建环境
func NewEnvironment(config *RLConfig) *Environment {
	return &Environment{
		config: config,
	}
}

// Reset 重置环境
func (env *Environment) Reset(metrics []*models.UAVMetrics) *State {
	if len(metrics) == 0 {
		return nil
	}

	// 初始化 GPS 转换器
	env.gpsConverter = NewGPSConverter(metrics[0].GPS.Latitude, metrics[0].GPS.Longitude)

	// 转换节点信息
	env.allNodes = make([]*NodeInfo, len(metrics))
	for i, m := range metrics {
		x, y := env.gpsConverter.GPSToXY(m.GPS.Latitude, m.GPS.Longitude)
		env.allNodes[i] = &NodeInfo{
			Metrics: m,
			XMeters: x,
			YMeters: y,
		}
	}

	// 计算节点特征
	env.computeNodeFeatures()

	// 初始化选择状态
	env.selectedNodes = []*NodeInfo{}
	env.selectedMask = make([]bool, len(env.allNodes))

	// 计算绘图区域和网格
	env.plotArea = env.calculatePlotArea()
	env.gridPoints = env.generateGridPoints()

	// 计算最大覆盖面积
	env.maxCoverage = env.calculateCoverage(env.allNodes)
	env.currentCoverage = 0

	return env.getState()
}

// Step 执行一步
func (env *Environment) Step(action Action) (*State, float64, bool) {
	if action.NodeIndex < 0 || action.NodeIndex >= len(env.allNodes) {
		// 无效动作或停止
		return env.getState(), 0, true
	}

	if env.selectedMask[action.NodeIndex] {
		// 已选择的节点，给予惩罚
		return env.getState(), -1.0, false
	}

	// 选择节点
	node := env.allNodes[action.NodeIndex]
	env.selectedNodes = append(env.selectedNodes, node)
	env.selectedMask[action.NodeIndex] = true

	// 计算新覆盖率
	prevCoverage := env.currentCoverage
	env.currentCoverage = env.calculateCoverageRatio()

	// 计算奖励
	reward := env.calculateReward(node, prevCoverage, env.currentCoverage)

	// 检查是否完成
	done := env.currentCoverage >= env.config.TargetCoverage ||
		len(env.selectedNodes) >= len(env.allNodes)

	// 达到目标覆盖率的额外奖励
	if env.currentCoverage >= env.config.TargetCoverage && prevCoverage < env.config.TargetCoverage {
		reward += env.config.TargetBonus
	}

	return env.getState(), reward, done
}

// getState 获取当前状态
func (env *Environment) getState() *State {
	nodeFeatures := make([]NodeFeatures, len(env.allNodes))
	selectionMask := make([]float64, len(env.allNodes))

	for i, node := range env.allNodes {
		nodeFeatures[i] = node.Features

		if env.selectedMask[i] {
			selectionMask[i] = 0 // 已选择
		} else {
			selectionMask[i] = 1 // 可选择
		}
	}

	return &State{
		NodeFeatures:    nodeFeatures,
		CurrentCoverage: env.currentCoverage,
		SelectedCount:   len(env.selectedNodes),
		TotalNodes:      len(env.allNodes),
		TargetCoverage:  env.config.TargetCoverage,
		SelectionMask:   selectionMask,
	}
}

// computeNodeFeatures 计算节点特征
func (env *Environment) computeNodeFeatures() {
	if len(env.allNodes) == 0 {
		return
	}

	// 计算边界和中心
	xMin, xMax := env.allNodes[0].XMeters, env.allNodes[0].XMeters
	yMin, yMax := env.allNodes[0].YMeters, env.allNodes[0].YMeters

	for _, node := range env.allNodes {
		if node.XMeters < xMin {
			xMin = node.XMeters
		}
		if node.XMeters > xMax {
			xMax = node.XMeters
		}
		if node.YMeters < yMin {
			yMin = node.YMeters
		}
		if node.YMeters > yMax {
			yMax = node.YMeters
		}
	}

	centerX := (xMin + xMax) / 2
	centerY := (yMin + yMax) / 2
	rangeX := xMax - xMin
	rangeY := yMax - yMin
	maxRange := math.Max(rangeX, rangeY)
	if maxRange == 0 {
		maxRange = 1
	}

	// 计算最大延迟 (用于归一化)
	maxLatency := 1.0
	for _, node := range env.allNodes {
		if node.Metrics.Network != nil && node.Metrics.Network.Latency > maxLatency {
			maxLatency = node.Metrics.Network.Latency
		}
	}

	// 为每个节点计算特征
	for i, node := range env.allNodes {
		// 归一化坐标
		normX := (node.XMeters - xMin) / maxRange
		normY := (node.YMeters - yMin) / maxRange

		// 到中心的距离
		distToCenter := math.Sqrt(math.Pow(node.XMeters-centerX, 2) + math.Pow(node.YMeters-centerY, 2))
		normDistToCenter := distToCenter / (maxRange / 2)
		if normDistToCenter > 1 {
			normDistToCenter = 1
		}

		// 最近邻距离
		minNeighborDist := math.MaxFloat64
		for j, other := range env.allNodes {
			if i != j {
				dist := math.Sqrt(math.Pow(node.XMeters-other.XMeters, 2) + math.Pow(node.YMeters-other.YMeters, 2))
				if dist < minNeighborDist {
					minNeighborDist = dist
				}
			}
		}
		normNeighborDist := minNeighborDist / env.config.CoverageRadius
		if normNeighborDist > 1 {
			normNeighborDist = 1
		}

		// 资源特征
		battery := node.Metrics.Battery.RemainingPercent / 100.0
		latency := 0.0
		if node.Metrics.Network != nil {
			latency = node.Metrics.Network.Latency / maxLatency
		}
		cpuUsage := 0.0
		memUsage := 0.0
		if node.Metrics.Performance != nil {
			cpuUsage = node.Metrics.Performance.CPUUsage / 100.0
			memUsage = node.Metrics.Performance.MemoryUsage / 100.0
		}

		env.allNodes[i].Features = NodeFeatures{
			NormX:             normX,
			NormY:             normY,
			Battery:          battery,
			Latency:          latency,
			CPUUsage:         cpuUsage,
			MemoryUsage:      memUsage,
			DistanceToCenter: normDistToCenter,
			NearestNeighborDist: normNeighborDist,
		}
	}
}

// calculatePlotArea 计算绘图区域
func (env *Environment) calculatePlotArea() PlotArea {
	if len(env.allNodes) == 0 {
		return PlotArea{-1000, 1000, -1000, 1000}
	}

	xMin, xMax := env.allNodes[0].XMeters, env.allNodes[0].XMeters
	yMin, yMax := env.allNodes[0].YMeters, env.allNodes[0].YMeters

	for _, node := range env.allNodes {
		if node.XMeters < xMin {
			xMin = node.XMeters
		}
		if node.XMeters > xMax {
			xMax = node.XMeters
		}
		if node.YMeters < yMin {
			yMin = node.YMeters
		}
		if node.YMeters > yMax {
			yMax = node.YMeters
		}
	}

	margin := env.config.CoverageRadius * 1.2
	return PlotArea{
		XMin: xMin - margin,
		XMax: xMax + margin,
		YMin: yMin - margin,
		YMax: yMax + margin,
	}
}

// generateGridPoints 生成网格点
func (env *Environment) generateGridPoints() [][2]float64 {
	density := env.config.GridDensity
	points := make([][2]float64, 0, density*density)

	width := env.plotArea.XMax - env.plotArea.XMin
	height := env.plotArea.YMax - env.plotArea.YMin
	stepX := width / float64(density)
	stepY := height / float64(density)

	for i := 0; i < density; i++ {
		for j := 0; j < density; j++ {
			x := env.plotArea.XMin + (float64(i)+0.5)*stepX
			y := env.plotArea.YMin + (float64(j)+0.5)*stepY
			points = append(points, [2]float64{x, y})
		}
	}

	return points
}

// calculateCoverage 计算覆盖面积
func (env *Environment) calculateCoverage(nodes []*NodeInfo) float64 {
	if len(nodes) == 0 || len(env.gridPoints) == 0 {
		return 0
	}

	width := env.plotArea.XMax - env.plotArea.XMin
	height := env.plotArea.YMax - env.plotArea.YMin
	cellArea := (width / float64(env.config.GridDensity)) * (height / float64(env.config.GridDensity))

	coveredCount := 0
	radiusSquared := env.config.CoverageRadius * env.config.CoverageRadius

	for _, point := range env.gridPoints {
		for _, node := range nodes {
			dx := point[0] - node.XMeters
			dy := point[1] - node.YMeters
			if dx*dx+dy*dy <= radiusSquared {
				coveredCount++
				break
			}
		}
	}

	return float64(coveredCount) * cellArea
}

// calculateCoverageRatio 计算覆盖率
func (env *Environment) calculateCoverageRatio() float64 {
	if env.maxCoverage == 0 {
		return 0
	}
	currentArea := env.calculateCoverage(env.selectedNodes)
	return currentArea / env.maxCoverage
}

// calculateReward 计算奖励 (优化版V2：更激进的节点效率优化)
func (env *Environment) calculateReward(node *NodeInfo, prevCoverage, newCoverage float64) float64 {
	reward := 0.0
	coverageGain := newCoverage - prevCoverage
	nodeCount := len(env.selectedNodes)
	totalNodes := len(env.allNodes)
	targetCov := env.config.TargetCoverage

	// 1. 覆盖率增量奖励 - 按边际效率加权
	if coverageGain > 0 {
		// 基础奖励
		reward += coverageGain * env.config.CoverageRewardScale

		// 边际效率奖励：贡献越高，奖励越多
		expectedGain := targetCov / float64(totalNodes) // 平均每个节点应贡献
		efficiency := coverageGain / expectedGain
		if efficiency > 1.5 {
			reward += 1.0 * (efficiency - 1.0) // 高效节点额外奖励
		} else if efficiency < 0.5 {
			reward -= 0.5 // 低效节点惩罚
		}
	} else {
		// 0覆盖贡献，严重惩罚
		reward -= 2.0
	}

	// 2. 动态节点惩罚：基于当前进度
	basePenalty := env.config.NodePenalty
	progress := newCoverage / targetCov
	if progress >= 1.0 {
		// 已达标，极大惩罚 (不应该再选)
		basePenalty *= 10.0
	} else if progress >= 0.95 {
		// 接近目标，大惩罚
		basePenalty *= 4.0
	} else if progress >= 0.85 {
		basePenalty *= 2.5
	} else if progress >= 0.70 {
		basePenalty *= 1.5
	}
	reward -= basePenalty

	// 3. 节点使用率惩罚：使用越多节点，惩罚越大
	nodeRatio := float64(nodeCount) / float64(totalNodes)
	reward -= nodeRatio * 0.5 // 使用50%节点时额外扣0.25分

	// 4. 达到目标的奖励 (节点越少奖励越大)
	if newCoverage >= targetCov && prevCoverage < targetCov {
		// 用更少的节点达标 = 更高奖励
		efficiencyBonus := (1.0 - nodeRatio) * env.config.TargetBonus * 2
		reward += efficiencyBonus
	}

	// 5. 远离已选节点的奖励（减少重叠）
	if len(env.selectedNodes) > 1 {
		minDist := math.MaxFloat64
		for _, selected := range env.selectedNodes[:len(env.selectedNodes)-1] {
			dist := math.Sqrt(math.Pow(node.XMeters-selected.XMeters, 2) +
				math.Pow(node.YMeters-selected.YMeters, 2))
			if dist < minDist {
				minDist = dist
			}
		}
		// 距离越远（重叠越少），奖励越高
		if minDist > env.config.CoverageRadius*1.5 {
			reward += 0.3
		} else if minDist < env.config.CoverageRadius*0.5 {
			reward -= 0.3 // 重叠太多
		}
	}

	return reward
}

// GetSelectedNodes 获取已选择的节点名称
func (env *Environment) GetSelectedNodes() []string {
	names := make([]string, len(env.selectedNodes))
	for i, node := range env.selectedNodes {
		names[i] = node.Metrics.NodeName
	}
	return names
}

// GetCurrentCoverage 获取当前覆盖率
func (env *Environment) GetCurrentCoverage() float64 {
	return env.currentCoverage
}

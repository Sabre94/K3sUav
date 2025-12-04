package greed_nsgaii

import (
	"math"
)

// PlotArea 绘图区域（用于计算覆盖面积）
type PlotArea struct {
	XMin float64
	XMax float64
	YMin float64
	YMax float64
}

// CalculatePlotArea 根据节点列表计算绘图区域
// 自动扩展边界以包含所有节点的覆盖范围
func CalculatePlotArea(nodes []*NodeInfo, coverageRadius float64) PlotArea {
	if len(nodes) == 0 {
		return PlotArea{XMin: -1000, XMax: 1000, YMin: -1000, YMax: 1000}
	}

	// 找出所有节点的边界
	xMin, xMax := nodes[0].XMeters, nodes[0].XMeters
	yMin, yMax := nodes[0].YMeters, nodes[0].YMeters

	for _, node := range nodes {
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

	// 扩展边界以包含覆盖半径
	margin := coverageRadius * 1.2
	return PlotArea{
		XMin: xMin - margin,
		XMax: xMax + margin,
		YMin: yMin - margin,
		YMax: yMax + margin,
	}
}

// CalculateUnionArea 使用网格采样法计算多个节点的覆盖面积并集
func CalculateUnionArea(nodes []*NodeInfo, plotArea PlotArea, coverageRadius float64, gridDensity int) float64 {
	if len(nodes) == 0 {
		return 0.0
	}

	// 计算网格尺寸
	width := plotArea.XMax - plotArea.XMin
	height := plotArea.YMax - plotArea.YMin

	// 生成网格点
	gridPoints := generateGridPoints(plotArea, gridDensity)

	// 计算网格单元面积
	cellArea := (width / float64(gridDensity)) * (height / float64(gridDensity))

	// 统计被覆盖的网格点数量
	coveredCount := 0
	for _, point := range gridPoints {
		if isPointCovered(point, nodes, coverageRadius) {
			coveredCount++
		}
	}

	// 总覆盖面积 = 被覆盖的网格点数 × 单元面积
	return float64(coveredCount) * cellArea
}

// generateGridPoints 生成网格点
func generateGridPoints(plotArea PlotArea, gridDensity int) [][2]float64 {
	points := make([][2]float64, 0, gridDensity*gridDensity)

	width := plotArea.XMax - plotArea.XMin
	height := plotArea.YMax - plotArea.YMin

	stepX := width / float64(gridDensity)
	stepY := height / float64(gridDensity)

	for i := 0; i < gridDensity; i++ {
		for j := 0; j < gridDensity; j++ {
			x := plotArea.XMin + (float64(i)+0.5)*stepX
			y := plotArea.YMin + (float64(j)+0.5)*stepY
			points = append(points, [2]float64{x, y})
		}
	}

	return points
}

// isPointCovered 判断点是否被任一节点覆盖
func isPointCovered(point [2]float64, nodes []*NodeInfo, coverageRadius float64) bool {
	for _, node := range nodes {
		distance := EuclideanDistance(point[0], point[1], node.XMeters, node.YMeters)
		if distance <= coverageRadius {
			return true
		}
	}
	return false
}

// CalculateIncrementalArea 计算添加新节点后的增量覆盖面积
func CalculateIncrementalArea(newNode *NodeInfo, existingNodes []*NodeInfo, plotArea PlotArea, coverageRadius float64, gridDensity int) float64 {
	// 计算添加新节点前的覆盖面积
	areaBefore := CalculateUnionArea(existingNodes, plotArea, coverageRadius, gridDensity)

	// 计算添加新节点后的覆盖面积
	nodesWithNew := append(existingNodes, newNode)
	areaAfter := CalculateUnionArea(nodesWithNew, plotArea, coverageRadius, gridDensity)

	// 增量面积
	return areaAfter - areaBefore
}

// CalculateMaxPossibleArea 计算所有节点的最大可能覆盖面积
func CalculateMaxPossibleArea(allNodes []*NodeInfo, coverageRadius float64, gridDensity int) float64 {
	if len(allNodes) == 0 {
		return 0.0
	}

	plotArea := CalculatePlotArea(allNodes, coverageRadius)
	return CalculateUnionArea(allNodes, plotArea, coverageRadius, gridDensity)
}

// CalculateCoverageRatio 计算覆盖率（当前覆盖面积 / 最大可能覆盖面积）
func CalculateCoverageRatio(currentArea, maxArea float64) float64 {
	if maxArea == 0 {
		return 0.0
	}
	return currentArea / maxArea
}

// CalculateSingleNodeArea 计算单个节点的覆盖面积（圆形）
func CalculateSingleNodeArea(coverageRadius float64) float64 {
	return math.Pi * coverageRadius * coverageRadius
}

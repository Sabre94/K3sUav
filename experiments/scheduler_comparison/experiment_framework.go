package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"sort"

	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/greed_nsgaii"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// greedNSGAIIAdapter 适配器，用于将 greed_nsgaii.GreedNSGAIIAlgorithm 适配到 SchedulingAlgorithm 接口
type greedNSGAIIAdapter struct {
	algo *greed_nsgaii.GreedNSGAIIAlgorithm
}

func (a *greedNSGAIIAdapter) Name() string {
	return a.algo.Name()
}

func (a *greedNSGAIIAdapter) Filter(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]*models.UAVMetrics, error) {
	return a.algo.Filter(ctx, pod, metrics)
}

func (a *greedNSGAIIAdapter) Score(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]algorithm.NodeScore, error) {
	scores, err := a.algo.Score(ctx, pod, metrics)
	if err != nil {
		return nil, err
	}

	result := make([]algorithm.NodeScore, len(scores))
	for i, s := range scores {
		result[i] = algorithm.NodeScore{
			NodeName: s.NodeName,
			Score:    s.Score,
			Reason:   s.Reason,
		}
	}

	return result, nil
}

// NewGreedNSGAIIAdapter 创建适配器
func NewGreedNSGAIIAdapter(taskType greed_nsgaii.TaskType, targetCoverage, coverageRadius float64) algorithm.SchedulingAlgorithm {
	algo := greed_nsgaii.NewGreedNSGAIIAlgorithm(taskType, targetCoverage, coverageRadius)
	return &greedNSGAIIAdapter{algo: algo}
}

// ExperimentResult 实验结果
type ExperimentResult struct {
	AlgorithmName    string
	TaskType         string
	NumReplicas      int
	SelectedNodes    []string
	CoverageRatio    float64
	AvgBattery       float64
	AvgLatency       float64
	AvgCPUUsage      float64
	AvgMemoryUsage   float64
	NumNodesSelected int
}

// Experiment 实验配置
type Experiment struct {
	Name        string
	Description string
	TaskType    string
	Replicas    []int
	Algorithms  []AlgorithmConfig
}

// AlgorithmConfig 算法配置
type AlgorithmConfig struct {
	Name      string
	Algorithm algorithm.SchedulingAlgorithm
}

// LoadUAVMetrics 从 CSV 加载 UAV 数据
func LoadUAVMetrics(csvPath string) ([]*models.UAVMetrics, error) {
	file, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	// 跳过表头
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV file is empty")
	}

	metrics := make([]*models.UAVMetrics, 0, len(records)-1)

	for i, record := range records[1:] {
		if len(record) < 9 {
			continue
		}

		var nodeName string
		var lat, lon, battery, latency, cpu, memory float64

		fmt.Sscanf(record[0], "%s", &nodeName)
		fmt.Sscanf(record[1], "%f", &lat)
		fmt.Sscanf(record[2], "%f", &lon)
		fmt.Sscanf(record[5], "%f", &battery)
		fmt.Sscanf(record[6], "%f", &latency)
		fmt.Sscanf(record[7], "%f", &cpu)
		fmt.Sscanf(record[8], "%f", &memory)

		metrics = append(metrics, &models.UAVMetrics{
			NodeName: nodeName,
			GPS: models.GPSData{
				Latitude:  lat,
				Longitude: lon,
			},
			Battery: models.BatteryData{
				RemainingPercent: battery,
			},
			Network: &models.NetworkData{
				Latency: latency,
			},
			Performance: &models.PerformanceData{
				CPUUsage:    cpu,
				MemoryUsage: memory,
			},
		})

		_ = i // 避免未使用变量警告
	}

	return metrics, nil
}

// RunExperiment 运行单个实验
func RunExperiment(
	algo algorithm.SchedulingAlgorithm,
	metrics []*models.UAVMetrics,
	numReplicas int,
	taskType string,
) (*ExperimentResult, error) {
	ctx := context.Background()

	// 模拟为每个 replica 调度 Pod
	selectedNodes := []string{}
	selectedMetrics := []*models.UAVMetrics{}

	// 检查是否是 GREED-NSGAII 适配器
	greedAdapter, isGreedAlgo := algo.(*greedNSGAIIAdapter)

	for i := 0; i < numReplicas; i++ {
		// 创建模拟 Pod（设置 OwnerReferences 以模拟 Deployment）
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-pod-%d", i),
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "ReplicaSet",
						Name: "test-deployment",
					},
				},
			},
		}

		// 调用算法评分
		scores, err := algo.Score(ctx, pod, metrics)
		if err != nil {
			return nil, fmt.Errorf("score error: %w", err)
		}

		// 排序并选择最佳节点
		sort.Slice(scores, func(i, j int) bool {
			return scores[i].Score > scores[j].Score
		})

		if len(scores) == 0 {
			break
		}

		bestNode := scores[0].NodeName

		// 检查是否重复选择（避免重复）
		isDuplicate := false
		for _, selected := range selectedNodes {
			if selected == bestNode {
				isDuplicate = true
				break
			}
		}

		if isDuplicate {
			// 如果最佳节点已被选择，选择下一个
			found := false
			for _, score := range scores {
				if score.Score == 0 {
					// 跳过分数为0的节点
					continue
				}
				alreadySelected := false
				for _, selected := range selectedNodes {
					if selected == score.NodeName {
						alreadySelected = true
						break
					}
				}
				if !alreadySelected {
					bestNode = score.NodeName
					found = true
					break
				}
			}
			if !found {
				// 没有可选节点了
				break
			}
		}

		selectedNodes = append(selectedNodes, bestNode)

		// 找到该节点的 metrics
		var selectedMetric *models.UAVMetrics
		for _, m := range metrics {
			if m.NodeName == bestNode {
				selectedMetric = m
				selectedMetrics = append(selectedMetrics, m)
				break
			}
		}

		// **关键修复**: 如果是 GREED-NSGAII 算法，调用 RecordBinding 更新状态
		if isGreedAlgo && selectedMetric != nil {
			greedAdapter.algo.RecordBinding(pod, bestNode, metrics)
		}
	}

	// 计算指标
	result := &ExperimentResult{
		AlgorithmName:    algo.Name(),
		TaskType:         taskType,
		NumReplicas:      numReplicas,
		SelectedNodes:    selectedNodes,
		NumNodesSelected: len(selectedNodes),
	}

	if len(selectedMetrics) > 0 {
		var totalBattery, totalLatency, totalCPU, totalMemory float64
		for _, m := range selectedMetrics {
			totalBattery += m.Battery.RemainingPercent
			totalLatency += m.Network.Latency
			totalCPU += m.Performance.CPUUsage
			totalMemory += m.Performance.MemoryUsage
		}

		n := float64(len(selectedMetrics))
		result.AvgBattery = totalBattery / n
		result.AvgLatency = totalLatency / n
		result.AvgCPUUsage = totalCPU / n
		result.AvgMemoryUsage = totalMemory / n

		// 计算覆盖率（使用 greed_nsgaii 的覆盖计算方法）
		result.CoverageRatio = calculateCoverage(selectedMetrics, metrics)
	}

	return result, nil
}

// calculateCoverage 计算覆盖率
func calculateCoverage(selected, all []*models.UAVMetrics) float64 {
	if len(selected) == 0 || len(all) == 0 {
		return 0.0
	}

	// 简化计算：覆盖率 = 选中节点数 / 总节点数
	// 实际应该使用 greed_nsgaii 的覆盖面积计算方法
	// 但这里为了简单起见，使用节点数比例

	// 转换为 NodeInfo 格式
	coverageRadius := 200.0 // 米
	gridDensity := 50

	// 创建 GPS 转换器
	converter := greed_nsgaii.NewGPSConverter(all[0].GPS.Latitude, all[0].GPS.Longitude)

	// 转换所有节点
	allNodes := make([]*greed_nsgaii.NodeInfo, len(all))
	for i, m := range all {
		x, y := converter.GPSToXY(m.GPS.Latitude, m.GPS.Longitude)
		allNodes[i] = &greed_nsgaii.NodeInfo{
			Metrics: m,
			XMeters: x,
			YMeters: y,
		}
	}

	// 转换选中的节点
	selectedNodes := make([]*greed_nsgaii.NodeInfo, len(selected))
	for i, m := range selected {
		x, y := converter.GPSToXY(m.GPS.Latitude, m.GPS.Longitude)
		selectedNodes[i] = &greed_nsgaii.NodeInfo{
			Metrics: m,
			XMeters: x,
			YMeters: y,
		}
	}

	// 计算覆盖面积
	plotArea := greed_nsgaii.CalculatePlotArea(allNodes, coverageRadius)
	maxArea := greed_nsgaii.CalculateUnionArea(allNodes, plotArea, coverageRadius, gridDensity)
	selectedArea := greed_nsgaii.CalculateUnionArea(selectedNodes, plotArea, coverageRadius, gridDensity)

	if maxArea == 0 {
		return 0
	}

	return selectedArea / maxArea
}

// PrintResult 打印结果
func PrintResult(result *ExperimentResult) {
	fmt.Printf("  Algorithm: %s\n", result.AlgorithmName)
	fmt.Printf("    Coverage Ratio: %.2f%%\n", result.CoverageRatio*100)
	fmt.Printf("    Avg Battery: %.1f%%\n", result.AvgBattery)
	fmt.Printf("    Avg Latency: %.1fms\n", result.AvgLatency)
	fmt.Printf("    Avg CPU Usage: %.1f%%\n", result.AvgCPUUsage)
	fmt.Printf("    Avg Memory Usage: %.1f%%\n", result.AvgMemoryUsage)
	fmt.Printf("    Nodes Selected: %d/%d\n", result.NumNodesSelected, result.NumReplicas)
	fmt.Println()
}

// SaveResultsToCSV 保存结果到 CSV
func SaveResultsToCSV(results []*ExperimentResult, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入表头
	header := []string{
		"Algorithm", "TaskType", "NumReplicas", "CoverageRatio",
		"AvgBattery", "AvgLatency", "AvgCPUUsage", "AvgMemoryUsage", "NumNodesSelected",
	}
	writer.Write(header)

	// 写入数据
	for _, r := range results {
		record := []string{
			r.AlgorithmName,
			r.TaskType,
			fmt.Sprintf("%d", r.NumReplicas),
			fmt.Sprintf("%.4f", r.CoverageRatio),
			fmt.Sprintf("%.2f", r.AvgBattery),
			fmt.Sprintf("%.2f", r.AvgLatency),
			fmt.Sprintf("%.2f", r.AvgCPUUsage),
			fmt.Sprintf("%.2f", r.AvgMemoryUsage),
			fmt.Sprintf("%d", r.NumNodesSelected),
		}
		writer.Write(record)
	}

	return nil
}

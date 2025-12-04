package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	fmt.Println("==========================================")
	fmt.Println("Baseline 算法测试")
	fmt.Println("==========================================")
	fmt.Println()

	// 加载 UAV 数据
	csvPath := "data/uavmetrics.csv"
	metrics, err := LoadUAVMetrics(csvPath)
	if err != nil {
		log.Fatalf("Failed to load UAV metrics: %v", err)
	}

	fmt.Printf("✓ 加载 %d 个 UAV 节点\n\n", len(metrics))

	// 定义 Baseline 算法
	algorithms := []struct {
		Name      string
		Algorithm algorithm.SchedulingAlgorithm
		Desc      string
	}{
		{
			Name:      "K8s-Default",
			Algorithm: algorithm.NewLeastLoadedAlgorithm(),
			Desc:      "基于资源利用率（CPU + Memory）",
		},
		{
			Name:      "Latency-First",
			Algorithm: algorithm.NewNetworkLatencyAlgorithm(200.0), // 最大延迟 200ms
			Desc:      "基于网络延迟（延迟越低越好）",
		},
	}

	// 测试场景：选择 10 个节点
	numReplicas := 10

	fmt.Println("==========================================")
	fmt.Printf("测试场景：选择 %d 个节点部署 Pod\n", numReplicas)
	fmt.Println("==========================================")
	fmt.Println()

	for _, algoConfig := range algorithms {
		fmt.Printf("【%s】\n", algoConfig.Name)
		fmt.Printf("  算法描述: %s\n", algoConfig.Desc)
		fmt.Println()

		selectedNodes, selectedMetrics := runScheduling(algoConfig.Algorithm, metrics, numReplicas)

		// 统计选中节点的指标
		var totalBattery, totalLatency, totalCPU, totalMemory float64
		var minDistance, maxDistance float64
		minDistance = 999999.0

		for _, m := range selectedMetrics {
			totalBattery += m.Battery.RemainingPercent
			if m.Network != nil {
				totalLatency += m.Network.Latency
			}
			if m.Performance != nil {
				totalCPU += m.Performance.CPUUsage
				totalMemory += m.Performance.MemoryUsage
			}

			// 计算距离（通过 x, y 坐标）
			distance := getDistance(m, metrics[0]) // 假设第一个是中心
			if distance < minDistance {
				minDistance = distance
			}
			if distance > maxDistance {
				maxDistance = distance
			}
		}

		n := float64(len(selectedMetrics))
		fmt.Printf("  选中节点: %v\n", selectedNodes)
		fmt.Printf("  平均电量: %.1f%%\n", totalBattery/n)
		fmt.Printf("  平均延迟: %.2f ms\n", totalLatency/n)
		fmt.Printf("  平均CPU:  %.1f%%\n", totalCPU/n)
		fmt.Printf("  平均内存: %.1f%%\n", totalMemory/n)
		fmt.Printf("  距离范围: %.0f - %.0f 米\n", minDistance, maxDistance)
		fmt.Println()
	}

	fmt.Println("==========================================")
	fmt.Println("测试完成")
	fmt.Println("==========================================")
}

func runScheduling(algo algorithm.SchedulingAlgorithm, metrics []*models.UAVMetrics, numReplicas int) ([]string, []*models.UAVMetrics) {
	ctx := context.Background()
	selectedNodes := []string{}
	selectedMetrics := []*models.UAVMetrics{}
	selectedSet := make(map[string]bool)

	for i := 0; i < numReplicas; i++ {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-pod-%d", i),
				Namespace: "default",
			},
		}

		// 调用算法评分
		scores, err := algo.Score(ctx, pod, metrics)
		if err != nil {
			log.Printf("Score error: %v", err)
			continue
		}

		// 排序
		sort.Slice(scores, func(i, j int) bool {
			return scores[i].Score > scores[j].Score
		})

		// 选择未被选中的最高分节点
		var bestNode string
		for _, score := range scores {
			if !selectedSet[score.NodeName] && score.Score > 0 {
				bestNode = score.NodeName
				break
			}
		}

		if bestNode == "" {
			break
		}

		selectedNodes = append(selectedNodes, bestNode)
		selectedSet[bestNode] = true

		// 找到对应的 metrics
		for _, m := range metrics {
			if m.NodeName == bestNode {
				selectedMetrics = append(selectedMetrics, m)
				break
			}
		}
	}

	return selectedNodes, selectedMetrics
}

func getDistance(m *models.UAVMetrics, center *models.UAVMetrics) float64 {
	// 简化：通过 GPS 坐标差异估算距离
	// 实际应该用 x_meters, y_meters，但这里先简化
	latDiff := m.GPS.Latitude - center.GPS.Latitude
	lonDiff := m.GPS.Longitude - center.GPS.Longitude
	return (latDiff*latDiff + lonDiff*lonDiff) * 111000 * 111000 // 粗略估算
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

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV file is empty")
	}

	metrics := []*models.UAVMetrics{}
	for i, record := range records[1:] {
		if len(record) < 9 {
			continue
		}

		nodeName := record[0]
		lat := parseFloat(record[1])
		lon := parseFloat(record[2])
		battery := parseFloat(record[5])
		latency := parseFloat(record[6])
		cpu := parseFloat(record[7])
		memory := parseFloat(record[8])

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

		_ = i
	}

	return metrics, nil
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

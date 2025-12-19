package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/greed_nsgaii"
)

func main() {
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("NSGA-II 性能测试 - 不同节点数量")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println()

	// 测试不同节点数量
	nodeCounts := []int{5, 10, 15, 20, 30, 50, 100}

	fmt.Println("NSGA-II 配置:")
	fmt.Println("  种群大小:   50")
	fmt.Println("  代数:       30")
	fmt.Println("  交叉率:     0.9")
	fmt.Println("  变异率:     0.1")
	fmt.Println("  目标覆盖率: 90%")
	fmt.Println()

	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("%-10s | %-12s | %-15s | %-12s | %s\n",
		"节点数", "执行时间", "Pareto解数量", "最优节点数", "可行解?")
	fmt.Println(strings.Repeat("-", 70))

	rand.Seed(time.Now().UnixNano())

	for _, numNodes := range nodeCounts {
		// 生成模拟数据
		metrics := generateMetrics(numNodes)

		// 创建算法实例
		algo := greed_nsgaii.NewGreedNSGAIIAlgorithm(
			greed_nsgaii.TaskTypeDefault,
			0.90, // 90% 覆盖率
			500,  // 500米覆盖半径
		)

		// 执行 NSGA-II
		start := time.Now()
		result := algo.RunNSGA2Optimization(metrics)
		duration := time.Since(start)

		// 获取结果
		paretoSize := len(result.ParetoFront)
		bestNodes := 0
		feasible := "否"

		if result.BestSolution != nil {
			bestNodes = int(result.BestSolution.Objectives[3])
			if result.BestSolution.IsFeasible {
				feasible = "是"
			}
		}

		fmt.Printf("%-10d | %-12s | %-15d | %-12d | %s\n",
			numNodes, duration.Round(time.Millisecond), paretoSize, bestNodes, feasible)
	}

	fmt.Println(strings.Repeat("-", 70))
	fmt.Println()
	fmt.Println("说明:")
	fmt.Println("  - 执行时间随节点数增加而增长")
	fmt.Println("  - 复杂度主要来自覆盖率计算 (网格采样)")
	fmt.Println("  - 实际部署时建议节点数 < 50 以保证实时性")
}

// generateMetrics 生成模拟的 UAV 节点数据
func generateMetrics(numNodes int) []*models.UAVMetrics {
	metrics := make([]*models.UAVMetrics, numNodes)

	// 上海区域坐标范围
	baseLat := 31.0
	baseLon := 121.3

	for i := 0; i < numNodes; i++ {
		metrics[i] = &models.UAVMetrics{
			NodeName: fmt.Sprintf("uav-node-%d", i+1),
			GPS: models.GPSData{
				Latitude:   baseLat + rand.Float64()*0.5,
				Longitude:  baseLon + rand.Float64()*0.5,
				Altitude:   100 + rand.Float64()*50,
				LastUpdate: time.Now(),
			},
			Battery: models.BatteryData{
				RemainingPercent: 20 + rand.Float64()*80,
				Voltage:          11.1 + rand.Float64()*1.5,
				Temperature:      25 + rand.Float64()*15,
			},
			Network: &models.NetworkData{
				Latency:        20 + rand.Float64()*150,
				Bandwidth:      50 + rand.Float64()*50,
				PacketLoss:     rand.Float64() * 5,
				SignalStrength: int(-50 - rand.Float64()*40),
			},
			Performance: &models.PerformanceData{
				CPUUsage:    10 + rand.Float64()*70,
				MemoryUsage: 20 + rand.Float64()*60,
				DiskUsage:   10 + rand.Float64()*50,
			},
		}
	}

	return metrics
}

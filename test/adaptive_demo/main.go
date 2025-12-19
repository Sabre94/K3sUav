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
	fmt.Println("自适应覆盖率调度测试")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println()

	// 1. 生成初始节点数据
	rand.Seed(time.Now().UnixNano())
	numNodes := 15
	metrics := generateTestMetrics(numNodes)

	fmt.Printf("生成 %d 个模拟 UAV 节点\n", numNodes)
	fmt.Println()

	// 2. 创建自适应调度器
	config := &greed_nsgaii.AdaptiveConfig{
		TargetCoverageRatio:   0.90,  // 目标 90% 覆盖率
		MinCoverageRatio:      0.70,  // 最低 70%
		MinorDropThreshold:    0.10,  // 小幅下降 10%
		MajorDropThreshold:    0.30,  // 大幅下降 30%
		MonitorInterval:       5 * time.Second,
		NodeTimeoutDuration:   60 * time.Second,
		CoverageRadius:        500.0, // 500米
		GridDensity:           50,
	}

	scheduler := greed_nsgaii.NewAdaptiveScheduler(config, greed_nsgaii.TaskTypeDefault)

	fmt.Println("配置:")
	fmt.Printf("  目标覆盖率:     %.0f%%\n", config.TargetCoverageRatio*100)
	fmt.Printf("  最低覆盖率:     %.0f%%\n", config.MinCoverageRatio*100)
	fmt.Printf("  小幅下降阈值:   %.0f%%\n", config.MinorDropThreshold*100)
	fmt.Printf("  大幅下降阈值:   %.0f%%\n", config.MajorDropThreshold*100)
	fmt.Printf("  覆盖半径:       %.0f米\n", config.CoverageRadius)
	fmt.Println()

	// 3. 初始化 - 使用 NSGA-II 获取初始方案
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("阶段1: 初始规划 (NSGA-II)")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println()

	selectedNodes, result, err := scheduler.ExecuteNSGA2Replan("test-deployment", metrics)
	if err != nil {
		fmt.Printf("初始规划失败: %v\n", err)
		return
	}

	fmt.Printf("初始方案:\n")
	fmt.Printf("  选中节点数: %d\n", len(selectedNodes))
	fmt.Printf("  节点列表:   %v\n", selectedNodes)
	if result.BestSolution != nil {
		fmt.Printf("  平均电量:   %.2f%%\n", -result.BestSolution.Objectives[0])
		fmt.Printf("  平均延迟:   %.2fms\n", result.BestSolution.Objectives[1])
	}
	fmt.Println()

	// 初始化覆盖率跟踪
	scheduler.InitializeDeployment("test-deployment", selectedNodes, metrics)
	state := scheduler.GetState("test-deployment")
	fmt.Printf("初始覆盖率: %.2f%%\n", state.CurrentCoverage*100)
	fmt.Println()

	// 4. 模拟场景1: 小幅节点离线 (1-2个节点)
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("阶段2: 模拟小幅节点离线 (移除2个节点)")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println()

	// 移除2个节点
	if len(selectedNodes) >= 2 {
		scheduler.RemoveNode("test-deployment", selectedNodes[0])
		scheduler.RemoveNode("test-deployment", selectedNodes[1])
		fmt.Printf("移除节点: %s, %s\n", selectedNodes[0], selectedNodes[1])
	}

	// 检查并决定动作
	state, _ = scheduler.CheckAndDecide("test-deployment", metrics)
	printState(state)

	// 根据推荐动作执行
	if state.RecommendedAction == greed_nsgaii.ActionGreedy {
		fmt.Println("\n执行贪心补充...")
		newNodes, _ := scheduler.ExecuteGreedyRepair("test-deployment", metrics)
		fmt.Printf("补充节点: %v\n", newNodes)

		state = scheduler.GetState("test-deployment")
		fmt.Printf("补充后覆盖率: %.2f%%\n", state.CurrentCoverage*100)
		fmt.Printf("当前活跃节点: %v\n", state.ActiveNodes)
	} else if state.RecommendedAction == greed_nsgaii.ActionReplan {
		fmt.Println("\n执行 NSGA-II 重规划...")
		newNodes, _, _ := scheduler.ExecuteNSGA2Replan("test-deployment", metrics)
		fmt.Printf("新方案节点: %v\n", newNodes)
	}
	fmt.Println()

	// 5. 模拟场景2: 大幅节点离线 (移除多个节点)
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("阶段3: 模拟大幅节点离线 (移除5个节点)")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println()

	// 重新初始化
	selectedNodes2, _, _ := scheduler.ExecuteNSGA2Replan("test-deployment-2", metrics)
	scheduler.InitializeDeployment("test-deployment-2", selectedNodes2, metrics)

	// 移除5个节点（模拟大幅下降）
	for i := 0; i < 5 && i < len(selectedNodes2); i++ {
		scheduler.RemoveNode("test-deployment-2", selectedNodes2[i])
		fmt.Printf("移除节点: %s\n", selectedNodes2[i])
	}

	// 检查并决定动作
	state2, _ := scheduler.CheckAndDecide("test-deployment-2", metrics)
	printState(state2)

	// 根据推荐动作执行
	if state2.RecommendedAction == greed_nsgaii.ActionReplan {
		fmt.Println("\n执行 NSGA-II 重规划...")
		newNodes, result, _ := scheduler.ExecuteNSGA2Replan("test-deployment-2", metrics)
		fmt.Printf("新方案节点数: %d\n", len(newNodes))
		fmt.Printf("新方案节点:   %v\n", newNodes)
		if result.BestSolution != nil {
			fmt.Printf("平均电量:     %.2f%%\n", -result.BestSolution.Objectives[0])
		}
	} else if state2.RecommendedAction == greed_nsgaii.ActionGreedy {
		fmt.Println("\n执行贪心补充...")
		newNodes, _ := scheduler.ExecuteGreedyRepair("test-deployment-2", metrics)
		fmt.Printf("补充节点: %v\n", newNodes)
	}
	fmt.Println()

	// 6. 总结决策逻辑
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("决策逻辑总结")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println()

	fmt.Println("覆盖率变化    -> 动作")
	fmt.Println("-" + strings.Repeat("-", 40))
	fmt.Println("下降 < 10%    -> 无需操作 (ActionNone)")
	fmt.Println("10% ≤ 下降 < 30% -> 贪心补充 (ActionGreedy)")
	fmt.Println("下降 ≥ 30%    -> NSGA-II 重规划 (ActionReplan)")
	fmt.Println("覆盖率 < 70%  -> 强制重规划 (ActionReplan)")
	fmt.Println()

	fmt.Println("使用方式:")
	fmt.Println("  1. 调用 InitializeDeployment() 初始化跟踪")
	fmt.Println("  2. 定期调用 CheckAndDecide() 检查覆盖率")
	fmt.Println("  3. 根据 RecommendedAction 执行相应操作:")
	fmt.Println("     - ActionGreedy: 调用 ExecuteGreedyRepair()")
	fmt.Println("     - ActionReplan: 调用 ExecuteNSGA2Replan()")
	fmt.Println("  4. 或使用 StartMonitoring() 启动自动监控")
}

// generateTestMetrics 生成测试数据
func generateTestMetrics(numNodes int) []*models.UAVMetrics {
	metrics := make([]*models.UAVMetrics, numNodes)
	baseLat := 31.2304
	baseLon := 121.4737

	for i := 0; i < numNodes; i++ {
		latOffset := (rand.Float64() - 0.5) * 0.1
		lonOffset := (rand.Float64() - 0.5) * 0.1

		metrics[i] = &models.UAVMetrics{
			NodeName: fmt.Sprintf("uav-node-%d", i+1),
			GPS: models.GPSData{
				Latitude:   baseLat + latOffset,
				Longitude:  baseLon + lonOffset,
				Altitude:   100.0 + rand.Float64()*50.0,
				LastUpdate: time.Now(),
			},
			Battery: models.BatteryData{
				RemainingPercent: 30.0 + rand.Float64()*70.0,
				Voltage:          11.0 + rand.Float64()*2.0,
			},
			Network: &models.NetworkData{
				Latency:        20.0 + rand.Float64()*100.0,
				Bandwidth:      30.0 + rand.Float64()*70.0,
				SignalStrength: -80 + rand.Intn(30),
			},
			Performance: &models.PerformanceData{
				CPUUsage:    15.0 + rand.Float64()*60.0,
				MemoryUsage: 20.0 + rand.Float64()*50.0,
			},
		}
	}
	return metrics
}

// printState 打印状态
func printState(state *greed_nsgaii.CoverageState) {
	fmt.Println("当前状态:")
	fmt.Printf("  当前覆盖率:   %.2f%%\n", state.CurrentCoverage*100)
	fmt.Printf("  上次覆盖率:   %.2f%%\n", state.LastCoverage*100)
	fmt.Printf("  下降幅度:     %.2f%%\n", state.CoverageDropRate*100)
	fmt.Printf("  活跃节点数:   %d\n", len(state.ActiveNodes))
	fmt.Printf("  离线节点数:   %d\n", len(state.OfflineNodes))
	fmt.Printf("  推荐动作:     %s\n", state.RecommendedAction)
}

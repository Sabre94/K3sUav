package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/greed_nsgaii"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/rl_coverage"
)

func main() {
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println("RL Coverage vs NSGA-II 性能对比")
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println()

	rand.Seed(time.Now().UnixNano())

	// 测试配置
	numNodes := 30
	targetCoverage := 0.90

	fmt.Printf("测试配置:\n")
	fmt.Printf("  节点数量: %d\n", numNodes)
	fmt.Printf("  目标覆盖率: %.0f%%\n", targetCoverage*100)
	fmt.Println()

	// 生成测试数据
	_ = generateTestMetrics(numNodes)

	// ==================== 1. RL 训练 ====================
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println("阶段 1: RL 模型训练")
	fmt.Println("=" + strings.Repeat("=", 70))

	rlConfig := rl_coverage.DefaultRLConfig()
	rlConfig.TargetCoverage = targetCoverage
	rlConfig.GridDensity = 30 // 降低网格密度加速训练

	rlAlgo := rl_coverage.NewRLCoverageAlgorithm(rlConfig)

	// 生成训练数据
	trainingData := make([][]*models.UAVMetrics, 10)
	for i := 0; i < 10; i++ {
		trainingData[i] = generateTestMetrics(numNodes)
	}

	// 快速训练
	trainerConfig := &rl_coverage.TrainerConfig{
		NumEpisodes:     50, // 快速演示
		EvalInterval:    25,
		SaveInterval:    1000,
		LogInterval:     10,
		MinNodes:        20,
		MaxNodes:        40,
		NumTrainingSets: 5,
	}

	trainer := rl_coverage.NewTrainer(rlAlgo, trainerConfig)

	fmt.Println("开始训练 (50 episodes)...")
	trainStart := time.Now()
	trainer.Train(trainingData)
	trainDuration := time.Since(trainStart)
	fmt.Printf("训练耗时: %v\n", trainDuration.Round(time.Millisecond))
	fmt.Println()

	// ==================== 2. RL 推理 ====================
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println("阶段 2: RL 推理 vs Greedy vs NSGA-II")
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println()

	// 使用相同的测试数据
	testMetrics := generateTestMetrics(numNodes)

	// 2.1 RL 推理
	fmt.Println("2.1 RL 推理")
	fmt.Println(strings.Repeat("-", 50))

	rlStart := time.Now()
	rlNodes, rlCoverage, err := rlAlgo.SelectNodes(testMetrics)
	rlDuration := time.Since(rlStart)

	if err != nil {
		fmt.Printf("  错误: %v\n", err)
	} else {
		fmt.Printf("  执行时间: %v\n", rlDuration)
		fmt.Printf("  选中节点数: %d\n", len(rlNodes))
		fmt.Printf("  覆盖率: %.2f%%\n", rlCoverage*100)
	}
	fmt.Println()

	// 2.2 Greedy
	fmt.Println("2.2 Greedy 贪心算法")
	fmt.Println(strings.Repeat("-", 50))

	greedyConfig := &greed_nsgaii.CoverageConfig{
		TargetCoverageRatio: targetCoverage,
		CoverageRadius:      500,
		GridDensity:         30,
	}

	// 转换节点
	gpsConverter := greed_nsgaii.NewGPSConverter(testMetrics[0].GPS.Latitude, testMetrics[0].GPS.Longitude)
	scorer := greed_nsgaii.NewNodeScorer(greed_nsgaii.TaskTypeDefault)
	allNodes := make([]*greed_nsgaii.NodeInfo, len(testMetrics))
	for i, m := range testMetrics {
		x, y := gpsConverter.GPSToXY(m.GPS.Latitude, m.GPS.Longitude)
		allNodes[i] = &greed_nsgaii.NodeInfo{
			Metrics: m,
			Score:   scorer.CalculateScore(m),
			XMeters: x,
			YMeters: y,
		}
	}

	greedy := greed_nsgaii.NewGreedySelector(greedyConfig)

	greedyStart := time.Now()
	greedyResult, err := greedy.SelectNodes(allNodes)
	greedyDuration := time.Since(greedyStart)

	if err != nil {
		fmt.Printf("  错误: %v\n", err)
	} else {
		greedyNodeNames := make([]string, len(greedyResult.SelectedNodes))
		for i, n := range greedyResult.SelectedNodes {
			greedyNodeNames[i] = n.Metrics.NodeName
		}

		fmt.Printf("  执行时间: %v\n", greedyDuration)
		fmt.Printf("  选中节点数: %d\n", len(greedyResult.SelectedNodes))
		fmt.Printf("  覆盖率: %.2f%%\n", greedyResult.AchievedCoverage*100)
	}
	fmt.Println()

	// 2.3 NSGA-II
	fmt.Println("2.3 NSGA-II 多目标优化")
	fmt.Println(strings.Repeat("-", 50))

	nsga2Algo := greed_nsgaii.NewGreedNSGAIIAlgorithm(
		greed_nsgaii.TaskTypeDefault,
		targetCoverage,
		500,
	)

	nsga2Start := time.Now()
	nsga2Result := nsga2Algo.RunNSGA2Optimization(testMetrics)
	nsga2Duration := time.Since(nsga2Start)

	nsga2NodeCount := 0
	if nsga2Result.BestSolution != nil {
		nsga2NodeCount = int(nsga2Result.BestSolution.Objectives[3])
	}

	fmt.Printf("  执行时间: %v\n", nsga2Duration)
	fmt.Printf("  选中节点数: %d\n", nsga2NodeCount)
	fmt.Printf("  Pareto 解数: %d\n", len(nsga2Result.ParetoFront))
	fmt.Println()

	// ==================== 3. 性能对比 ====================
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println("阶段 3: 性能对比汇总")
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println()

	greedyNodeCount := 0
	greedyCoverage := 0.0
	if greedyResult != nil {
		greedyNodeCount = len(greedyResult.SelectedNodes)
		greedyCoverage = greedyResult.AchievedCoverage
	}

	fmt.Printf("%-15s | %-12s | %-10s | %s\n", "算法", "执行时间", "节点数", "覆盖率")
	fmt.Println(strings.Repeat("-", 55))
	fmt.Printf("%-15s | %-12v | %-10d | %.2f%%\n", "RL", rlDuration, len(rlNodes), rlCoverage*100)
	fmt.Printf("%-15s | %-12v | %-10d | %.2f%%\n", "Greedy", greedyDuration, greedyNodeCount, greedyCoverage*100)
	fmt.Printf("%-15s | %-12v | %-10d | -\n", "NSGA-II", nsga2Duration, nsga2NodeCount)
	fmt.Println()

	// 加速比
	fmt.Println("加速比:")
	fmt.Printf("  RL vs NSGA-II: %.1fx 更快\n", float64(nsga2Duration)/float64(rlDuration))
	fmt.Printf("  Greedy vs NSGA-II: %.1fx 更快\n", float64(nsga2Duration)/float64(greedyDuration))
	fmt.Println()

	// ==================== 4. 结论 ====================
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println("结论")
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println()
	fmt.Println("1. RL 推理速度与 Greedy 相当 (毫秒级)")
	fmt.Println("2. 训练后的 RL 模型可以快速给出接近最优的解")
	fmt.Println("3. 推荐使用场景:")
	fmt.Println("   - 在线调度: RL 或 Greedy")
	fmt.Println("   - 离线优化: NSGA-II")
	fmt.Println("   - 大规模部署: 预训练 RL 模型")
}

func generateTestMetrics(numNodes int) []*models.UAVMetrics {
	metrics := make([]*models.UAVMetrics, numNodes)

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

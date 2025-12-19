package main

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/greed_nsgaii"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SchedulingResult 调度结果
type SchedulingResult struct {
	AlgorithmName   string
	SelectedNodes   []string  // 选中的节点列表（按分数排序）
	Scores          []float64 // 对应的分数
	Reasons         []string  // 选择原因
	TotalNodes      int
	ExecutionTimeMs int64
}

func main() {
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("UAV 调度算法独立测试")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println()

	// 1. 生成模拟的 UAV 节点数据
	numNodes := 10
	metrics := generateMockUAVMetrics(numNodes)

	fmt.Printf("生成了 %d 个模拟 UAV 节点:\n", numNodes)
	fmt.Println("-" + strings.Repeat("-", 79))
	printMetricsSummary(metrics)
	fmt.Println()

	// 2. 创建模拟的 Pod（用于触发调度）
	pod := createMockPod("test-deployment", "test-pod-1")

	// 3. 测试各种调度算法
	ctx := context.Background()

	// 目标位置（用于 distance-based 算法）
	targetLat := 31.2304  // 上海
	targetLon := 121.4737

	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("调度算法测试结果")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println()

	// 3.1 Distance-Based 算法
	distanceAlgo := algorithm.NewDistanceBasedAlgorithm(targetLat, targetLon)
	result1 := runAlgorithm(ctx, distanceAlgo, pod, metrics)
	printResult(result1)

	// 3.2 Battery-Aware 算法
	batteryAlgo := algorithm.NewBatteryAwareAlgorithm(30.0) // 最低 30% 电量
	result2 := runAlgorithm(ctx, batteryAlgo, pod, metrics)
	printResult(result2)

	// 3.3 Network-Latency 算法
	networkAlgo := algorithm.NewNetworkLatencyAlgorithm(100.0) // 最大 100ms 延迟
	result3 := runAlgorithm(ctx, networkAlgo, pod, metrics)
	printResult(result3)

	// 3.4 Least-Loaded 算法
	leastLoadedAlgo := algorithm.NewLeastLoadedAlgorithm()
	result4 := runAlgorithm(ctx, leastLoadedAlgo, pod, metrics)
	printResult(result4)

	// 3.5 Coverage-Based 算法
	coverageAlgo := algorithm.NewCoverageBasedAlgorithm(90.0, 5.0) // 90% 覆盖率，5km 半径
	result5 := runAlgorithm(ctx, coverageAlgo, pod, metrics)
	printResult(result5)

	// 3.6 GREED-NSGAII 算法
	greedNsgaiiAlgo := greed_nsgaii.NewGreedNSGAIIAlgorithm(
		greed_nsgaii.TaskTypeDefault,
		0.9,   // 90% 目标覆盖率
		200.0, // 200米覆盖半径
	)
	result6 := runGreedNSGAIIAlgorithm(ctx, greedNsgaiiAlgo, pod, metrics)
	printResult(result6)

	// 4. 模拟多 Pod 调度（覆盖率算法的贪心特性）
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("多 Pod 调度模拟 (GREED-NSGAII Greedy Phase)")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println()

	testMultiPodScheduling(ctx, metrics, 5) // 调度 5 个 Pod

	// 5. 执行完整的 NSGA-II 优化
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("NSGA-II 多目标优化 (离线批量优化)")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println()

	testNSGA2Optimization(metrics)

	// 6. 输出最终推荐
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("调度推荐汇总")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println()

	printFinalRecommendation([]SchedulingResult{result1, result2, result3, result4, result5, result6})
}

// generateMockUAVMetrics 生成模拟的 UAV 节点指标数据
func generateMockUAVMetrics(numNodes int) []*models.UAVMetrics {
	rand.Seed(time.Now().UnixNano())
	metrics := make([]*models.UAVMetrics, numNodes)

	// 基准位置（上海附近）
	baseLat := 31.2304
	baseLon := 121.4737

	for i := 0; i < numNodes; i++ {
		// 在基准位置周围随机分布（约 50km 范围内）
		latOffset := (rand.Float64() - 0.5) * 0.5  // ±0.25 度 ≈ ±27.5km
		lonOffset := (rand.Float64() - 0.5) * 0.5

		metrics[i] = &models.UAVMetrics{
			NodeName: fmt.Sprintf("uav-node-%d", i+1),
			GPS: models.GPSData{
				Latitude:   baseLat + latOffset,
				Longitude:  baseLon + lonOffset,
				Altitude:   100.0 + rand.Float64()*50.0,
				Heading:    rand.Float64() * 360.0,
				Speed:      rand.Float64() * 20.0,
				Satellites: 8 + rand.Intn(8),
				LastUpdate: time.Now(),
			},
			Battery: models.BatteryData{
				RemainingPercent: 20.0 + rand.Float64()*80.0, // 20-100%
				Voltage:          11.0 + rand.Float64()*2.0,
				Temperature:      25.0 + rand.Float64()*20.0,
				TimeRemaining:    600 + rand.Intn(1800),
			},
			Network: &models.NetworkData{
				Latency:        10.0 + rand.Float64()*150.0, // 10-160ms
				Bandwidth:      20.0 + rand.Float64()*80.0,  // 20-100 Mbps
				SignalStrength: -90 + rand.Intn(40),         // -90 to -50 dBm
				PacketLoss:     rand.Float64() * 5.0,        // 0-5%
				ConnectionType: []string{"4G", "5G", "WIFI"}[rand.Intn(3)],
			},
			Performance: &models.PerformanceData{
				CPUUsage:    10.0 + rand.Float64()*70.0, // 10-80%
				MemoryUsage: 20.0 + rand.Float64()*60.0, // 20-80%
				DiskUsage:   10.0 + rand.Float64()*50.0,
				Temperature: 40.0 + rand.Float64()*30.0,
			},
		}
	}

	return metrics
}

// createMockPod 创建模拟的 Pod
func createMockPod(deploymentName, podName string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: deploymentName + "-abc123",
				},
			},
			Annotations: map[string]string{
				"uav.scheduler/algorithm": "greed-nsgaii",
			},
		},
		Spec: v1.PodSpec{
			SchedulerName: "uav-scheduler",
		},
	}
}

// runAlgorithm 运行调度算法并返回结果
func runAlgorithm(ctx context.Context, algo algorithm.SchedulingAlgorithm, pod *v1.Pod, metrics []*models.UAVMetrics) SchedulingResult {
	start := time.Now()

	// 1. 过滤
	filteredMetrics, err := algo.Filter(ctx, pod, metrics)
	if err != nil {
		return SchedulingResult{
			AlgorithmName: algo.Name(),
			SelectedNodes: []string{},
			TotalNodes:    len(metrics),
		}
	}

	// 2. 评分
	scores, err := algo.Score(ctx, pod, filteredMetrics)
	if err != nil {
		return SchedulingResult{
			AlgorithmName: algo.Name(),
			SelectedNodes: []string{},
			TotalNodes:    len(metrics),
		}
	}

	elapsed := time.Since(start)

	// 3. 按分数排序
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	// 4. 提取结果
	selectedNodes := make([]string, len(scores))
	scoreValues := make([]float64, len(scores))
	reasons := make([]string, len(scores))

	for i, s := range scores {
		selectedNodes[i] = s.NodeName
		scoreValues[i] = s.Score
		reasons[i] = s.Reason
	}

	return SchedulingResult{
		AlgorithmName:   algo.Name(),
		SelectedNodes:   selectedNodes,
		Scores:          scoreValues,
		Reasons:         reasons,
		TotalNodes:      len(metrics),
		ExecutionTimeMs: elapsed.Milliseconds(),
	}
}

// runGreedNSGAIIAlgorithm 运行 GREED-NSGAII 算法
func runGreedNSGAIIAlgorithm(ctx context.Context, algo *greed_nsgaii.GreedNSGAIIAlgorithm, pod *v1.Pod, metrics []*models.UAVMetrics) SchedulingResult {
	start := time.Now()

	// 评分
	scores, err := algo.Score(ctx, pod, metrics)
	if err != nil {
		return SchedulingResult{
			AlgorithmName: algo.Name(),
			SelectedNodes: []string{},
			TotalNodes:    len(metrics),
		}
	}

	elapsed := time.Since(start)

	// 转换为 algorithm.NodeScore 进行排序
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	selectedNodes := make([]string, len(scores))
	scoreValues := make([]float64, len(scores))
	reasons := make([]string, len(scores))

	for i, s := range scores {
		selectedNodes[i] = s.NodeName
		scoreValues[i] = s.Score
		reasons[i] = s.Reason
	}

	return SchedulingResult{
		AlgorithmName:   algo.Name(),
		SelectedNodes:   selectedNodes,
		Scores:          scoreValues,
		Reasons:         reasons,
		TotalNodes:      len(metrics),
		ExecutionTimeMs: elapsed.Milliseconds(),
	}
}

// testNSGA2Optimization 测试 NSGA-II 多目标优化
func testNSGA2Optimization(metrics []*models.UAVMetrics) {
	fmt.Println("执行 NSGA-II 多目标优化...")
	fmt.Println("目标函数:")
	fmt.Println("  - Obj1: 最大化平均电量 (最小化负值)")
	fmt.Println("  - Obj2: 最小化平均延迟")
	fmt.Println("  - Obj3: 最小化平均利用率")
	fmt.Println("  - Obj4: 最小化节点数量")
	fmt.Println("约束: 覆盖率 >= 90%")
	fmt.Println()

	// 创建算法实例
	algo := greed_nsgaii.NewGreedNSGAIIAlgorithm(
		greed_nsgaii.TaskTypeDefault,
		0.9,    // 90% 目标覆盖率
		500.0,  // 500米覆盖半径
	)

	// 先调用一次 Score 来初始化 GPS 转换器
	ctx := context.Background()
	mockPod := createMockPod("nsga2-test", "nsga2-pod")
	algo.Score(ctx, mockPod, metrics)

	// 执行 NSGA-II 优化
	start := time.Now()
	result := algo.RunNSGA2Optimization(metrics)
	elapsed := time.Since(start)

	fmt.Printf("优化完成 (耗时: %dms, 代数: 30, 种群: 50)\n", elapsed.Milliseconds())
	fmt.Println()

	// 打印 Pareto 前沿
	fmt.Printf("Pareto 前沿大小: %d 个非支配解\n", len(result.ParetoFront))
	fmt.Println("-" + strings.Repeat("-", 79))
	fmt.Printf("%-4s | %-10s | %-10s | %-10s | %-10s | %-8s | %s\n",
		"No.", "平均电量", "平均延迟", "平均利用率", "节点数", "可行", "选中节点")
	fmt.Println("-" + strings.Repeat("-", 79))

	displayCount := 5
	if len(result.ParetoFront) < displayCount {
		displayCount = len(result.ParetoFront)
	}

	for i := 0; i < displayCount; i++ {
		ind := result.ParetoFront[i]
		// 提取选中的节点名称
		selectedNames := []string{}
		for j, selected := range ind.Chromosome {
			if selected && j < len(metrics) {
				selectedNames = append(selectedNames, metrics[j].NodeName)
			}
		}

		feasible := "否"
		if ind.IsFeasible {
			feasible = "是"
		}

		fmt.Printf("%-4d | %8.2f%% | %8.2fms | %8.2f%% | %8.0f | %-8s | %v\n",
			i+1,
			-ind.Objectives[0], // 取负值还原为正电量
			ind.Objectives[1],
			ind.Objectives[2],
			ind.Objectives[3],
			feasible,
			selectedNames)
	}
	fmt.Println()

	// 打印推荐解
	if result.BestSolution != nil {
		fmt.Println("NSGA-II 推荐解 (拥挤度最大的解):")
		fmt.Println("-" + strings.Repeat("-", 60))

		selectedNames := []string{}
		for j, selected := range result.BestSolution.Chromosome {
			if selected && j < len(metrics) {
				selectedNames = append(selectedNames, metrics[j].NodeName)
			}
		}

		fmt.Printf("  平均电量:   %.2f%%\n", -result.BestSolution.Objectives[0])
		fmt.Printf("  平均延迟:   %.2fms\n", result.BestSolution.Objectives[1])
		fmt.Printf("  平均利用率: %.2f%%\n", result.BestSolution.Objectives[2])
		fmt.Printf("  节点数量:   %.0f\n", result.BestSolution.Objectives[3])
		fmt.Printf("  是否可行:   %v\n", result.BestSolution.IsFeasible)
		fmt.Printf("  选中节点:   %v\n", selectedNames)
		fmt.Println()
		fmt.Println("这些节点可以直接用于 K8s Pod Binding!")
	}
	fmt.Println()
}

// testMultiPodScheduling 测试多 Pod 调度
func testMultiPodScheduling(ctx context.Context, metrics []*models.UAVMetrics, numPods int) {
	algo := greed_nsgaii.NewGreedNSGAIIAlgorithm(
		greed_nsgaii.TaskTypeSustain, // 持续任务
		0.9,                          // 90% 目标覆盖率
		500.0,                        // 500米覆盖半径
	)

	deploymentName := "multi-pod-test-deployment"
	selectedNodes := []string{}

	for i := 0; i < numPods; i++ {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i+1),
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "ReplicaSet",
						Name: deploymentName + "-abc123",
					},
				},
			},
		}

		// 锁定 Deployment（模拟串行调度）
		algo.LockDeployment(deploymentName + "-abc123")

		// 评分
		scores, err := algo.Score(ctx, pod, metrics)
		if err != nil {
			algo.UnlockDeployment(deploymentName + "-abc123")
			continue
		}

		// 选择最高分的节点
		var bestNode string
		var bestScore float64 = -1
		for _, s := range scores {
			if s.Score > bestScore {
				bestScore = s.Score
				bestNode = s.NodeName
			}
		}

		if bestNode != "" {
			// 记录绑定
			algo.RecordBinding(pod, bestNode, metrics)
			selectedNodes = append(selectedNodes, bestNode)

			// 获取覆盖率信息
			coverage, maxArea, numNodes := algo.GetCoverageInfo(deploymentName + "-abc123")

			fmt.Printf("Pod %d -> 节点: %-12s | 分数: %6.2f | 覆盖率: %5.2f%% | 已选节点数: %d\n",
				i+1, bestNode, bestScore, coverage*100, numNodes)
			_ = maxArea // 避免未使用警告
		}

		algo.UnlockDeployment(deploymentName + "-abc123")
	}

	fmt.Println()
	fmt.Println("最终选中的节点列表（可用于后续绑定）:")
	fmt.Printf("  %v\n", selectedNodes)
	fmt.Println()
}

// printMetricsSummary 打印节点指标摘要
func printMetricsSummary(metrics []*models.UAVMetrics) {
	fmt.Printf("%-15s | %-10s | %-10s | %-8s | %-8s | %-8s\n",
		"节点名称", "纬度", "经度", "电量%", "延迟ms", "CPU%")
	fmt.Println(strings.Repeat("-", 75))

	for _, m := range metrics {
		latency := 0.0
		cpu := 0.0
		if m.Network != nil {
			latency = m.Network.Latency
		}
		if m.Performance != nil {
			cpu = m.Performance.CPUUsage
		}
		fmt.Printf("%-15s | %10.4f | %10.4f | %6.1f%% | %6.1fms | %6.1f%%\n",
			m.NodeName,
			m.GPS.Latitude,
			m.GPS.Longitude,
			m.Battery.RemainingPercent,
			latency,
			cpu)
	}
}

// printResult 打印单个算法的结果
func printResult(result SchedulingResult) {
	fmt.Printf("算法: %s (耗时: %dms)\n", result.AlgorithmName, result.ExecutionTimeMs)
	fmt.Println(strings.Repeat("-", 60))

	// 只显示前 5 个推荐节点
	displayCount := 5
	if len(result.SelectedNodes) < displayCount {
		displayCount = len(result.SelectedNodes)
	}

	fmt.Println("推荐节点 (Top 5):")
	for i := 0; i < displayCount; i++ {
		fmt.Printf("  %d. %-15s | 分数: %6.2f | %s\n",
			i+1,
			result.SelectedNodes[i],
			result.Scores[i],
			truncateString(result.Reasons[i], 40))
	}
	fmt.Println()
}

// printFinalRecommendation 打印最终推荐汇总
func printFinalRecommendation(results []SchedulingResult) {
	fmt.Println("各算法推荐的最佳节点:")
	fmt.Printf("%-20s | %-15s | %-8s\n", "算法", "最佳节点", "分数")
	fmt.Println(strings.Repeat("-", 50))

	for _, r := range results {
		if len(r.SelectedNodes) > 0 {
			fmt.Printf("%-20s | %-15s | %8.2f\n",
				r.AlgorithmName,
				r.SelectedNodes[0],
				r.Scores[0])
		}
	}

	fmt.Println()
	fmt.Println("说明:")
	fmt.Println("  - 这些节点可以直接用于 K8s Pod Binding")
	fmt.Println("  - 调用 k8sClientset.CoreV1().Pods(ns).Bind(ctx, binding, opts)")
	fmt.Println("  - binding.Target.Name = 上述推荐的节点名称")
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

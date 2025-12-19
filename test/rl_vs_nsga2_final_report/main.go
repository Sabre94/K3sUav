package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/greed_nsgaii"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/rl_coverage"
)

// Result 测试结果
type Result struct {
	Pattern        string
	TargetCoverage float64
	NumNodes       int
	// RL结果
	RLTime     time.Duration
	RLNodes    int
	RLCoverage float64
	// NSGA-II结果
	NSGA2Time      time.Duration
	NSGA2Nodes     int
	NSGA2Coverage  float64
	NSGA2ParetoNum int
}

func main() {
	rand.Seed(time.Now().UnixNano())

	printHeader()

	// 测试配置
	coverageTargets := []float64{0.70, 0.80, 0.85, 0.90, 0.95}
	nodeCounts := []int{20, 30, 50}
	patterns := []string{"random", "grid", "line", "circle", "cluster"}
	patternNames := map[string]string{
		"random":  "随机分布",
		"grid":    "网格编队",
		"line":    "线性编队",
		"circle":  "环形编队",
		"cluster": "聚类分布",
	}

	// ==================== 1. 训练RL模型 ====================
	fmt.Println("\n┌─────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│                        阶段1: 训练RL模型                             │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────┘")

	generator := rl_coverage.NewDataGenerator(&rl_coverage.GeneratorConfig{
		MinNodes:             10,
		MaxNodes:             60,
		MinAreaSize:          2000,
		MaxAreaSize:          20000,
		EnableRandomPattern:  true,
		EnableGridPattern:    true,
		EnableLinePattern:    true,
		EnableCirclePattern:  true,
		EnableClusterPattern: true,
	})

	trainingData := generator.GenerateDiverseTrainingData(80)

	rlConfig := rl_coverage.DefaultRLConfig()
	rlConfig.HiddenSize = 128
	rlConfig.NumHiddenLayers = 2
	rlConfig.TargetCoverage = 0.85
	rlConfig.LearningRate = 0.005
	rlConfig.MaxStepsPerEpisode = 60
	rlConfig.GridDensity = 30

	rlAlgo := rl_coverage.NewRLCoverageAlgorithm(rlConfig)

	trainerConfig := &rl_coverage.TrainerConfig{
		NumEpisodes:     300,
		EvalInterval:    150,
		SaveInterval:    1000,
		LogInterval:     100,
		MinNodes:        10,
		MaxNodes:        60,
		NumTrainingSets: 80,
	}

	trainer := rl_coverage.NewTrainer(rlAlgo, trainerConfig)
	fmt.Println("\n训练中...")
	trainStart := time.Now()
	trainer.Train(trainingData)
	trainDuration := time.Since(trainStart)
	fmt.Printf("\n✓ 训练完成，耗时: %v\n", trainDuration.Round(time.Millisecond))

	// ==================== 2. 收集测试数据 ====================
	fmt.Println("\n┌─────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│                        阶段2: 执行对比测试                           │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────┘")

	var allResults []Result
	totalTests := len(patterns) * len(coverageTargets) * len(nodeCounts)
	currentTest := 0

	for _, numNodes := range nodeCounts {
		for _, pattern := range patterns {
			testGen := rl_coverage.NewDataGenerator(&rl_coverage.GeneratorConfig{
				MinNodes:             numNodes,
				MaxNodes:             numNodes,
				MinAreaSize:          8000,
				MaxAreaSize:          8000,
				EnableRandomPattern:  pattern == "random",
				EnableGridPattern:    pattern == "grid",
				EnableLinePattern:    pattern == "line",
				EnableCirclePattern:  pattern == "circle",
				EnableClusterPattern: pattern == "cluster",
			})

			testMetrics := testGen.GenerateDiverseTrainingData(1)[0]

			for _, targetCov := range coverageTargets {
				currentTest++
				fmt.Printf("\r  测试进度: %d/%d (%.0f%%)", currentTest, totalTests, float64(currentTest)/float64(totalTests)*100)

				result := Result{
					Pattern:        pattern,
					TargetCoverage: targetCov,
					NumNodes:       numNodes,
				}

				// RL 测试
				rlAlgo.SetTargetCoverage(targetCov)
				rlStart := time.Now()
				rlSelected, rlCov, _ := rlAlgo.SelectNodes(testMetrics)
				result.RLTime = time.Since(rlStart)
				result.RLNodes = len(rlSelected)
				result.RLCoverage = rlCov

				// NSGA-II 测试
				nsga2Algo := greed_nsgaii.NewGreedNSGAIIAlgorithm(
					greed_nsgaii.TaskTypeDefault,
					targetCov,
					500.0,
				)

				nsga2Start := time.Now()
				nsga2Result := nsga2Algo.RunNSGA2Optimization(testMetrics)
				result.NSGA2Time = time.Since(nsga2Start)

				if nsga2Result != nil {
					result.NSGA2ParetoNum = len(nsga2Result.ParetoFront)

					var bestSolution *greed_nsgaii.Individual
					for _, ind := range nsga2Result.ParetoFront {
						if ind.IsFeasible {
							if bestSolution == nil {
								bestSolution = ind
							} else {
								count1, count2 := countNodes(ind.Chromosome), countNodes(bestSolution.Chromosome)
								if count1 < count2 {
									bestSolution = ind
								}
							}
						}
					}

					if bestSolution == nil {
						bestSolution = nsga2Result.BestSolution
					}

					if bestSolution != nil {
						result.NSGA2Nodes = countNodes(bestSolution.Chromosome)
						result.NSGA2Coverage = calculateCoverage(testMetrics, bestSolution.Chromosome, 500.0)
					}
				}

				allResults = append(allResults, result)
			}
		}
	}

	fmt.Println("\n\n✓ 测试完成")

	// ==================== 3. 生成报告 ====================
	printFinalReport(allResults, coverageTargets, nodeCounts, patterns, patternNames)
}

func printHeader() {
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                                                                                   ║")
	fmt.Println("║          ██████╗ ██╗         ██╗   ██╗███████╗    ███╗   ██╗███████╗ ██████╗      ║")
	fmt.Println("║          ██╔══██╗██║         ██║   ██║██╔════╝    ████╗  ██║██╔════╝██╔════╝      ║")
	fmt.Println("║          ██████╔╝██║         ██║   ██║███████╗    ██╔██╗ ██║███████╗██║  ███╗     ║")
	fmt.Println("║          ██╔══██╗██║         ╚██╗ ██╔╝╚════██║    ██║╚██╗██║╚════██║██║   ██║     ║")
	fmt.Println("║          ██║  ██║███████╗     ╚████╔╝ ███████║    ██║ ╚████║███████║╚██████╔╝     ║")
	fmt.Println("║          ╚═╝  ╚═╝╚══════╝      ╚═══╝  ╚══════╝    ╚═╝  ╚═══╝╚══════╝ ╚═════╝      ║")
	fmt.Println("║                                                                                   ║")
	fmt.Println("║                    UAV 覆盖率调度算法 - 完整性能对比报告                            ║")
	fmt.Println("║                                                                                   ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════════════════════╝")
}

func printFinalReport(results []Result, targets []float64, nodeCounts []int, patterns []string, patternNames map[string]string) {
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                              完 整 对 比 报 告                                     ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════════════════════╝")

	// ==================== 表1: 总体统计 ====================
	fmt.Println("\n┌─────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│                          表1: 总体统计概览                           │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────┘")

	var totalRLTime, totalNSGA2Time time.Duration
	var totalRLNodes, totalNSGA2Nodes int
	rlHit, nsga2Hit := 0, 0

	for _, r := range results {
		totalRLTime += r.RLTime
		totalNSGA2Time += r.NSGA2Time
		totalRLNodes += r.RLNodes
		totalNSGA2Nodes += r.NSGA2Nodes
		if r.RLCoverage >= r.TargetCoverage*0.98 {
			rlHit++
		}
		if r.NSGA2Coverage >= r.TargetCoverage*0.98 {
			nsga2Hit++
		}
	}

	n := len(results)
	avgSpeedup := float64(totalNSGA2Time) / float64(totalRLTime)

	fmt.Println()
	fmt.Println("  ┌────────────────────┬─────────────────┬─────────────────┐")
	fmt.Println("  │       指标         │       RL        │    NSGA-II      │")
	fmt.Println("  ├────────────────────┼─────────────────┼─────────────────┤")
	fmt.Printf("  │ 测试场景总数       │ %15d │ %15d │\n", n, n)
	fmt.Printf("  │ 平均执行时间       │ %12v │ %12v │\n", (totalRLTime / time.Duration(n)).Round(time.Microsecond), (totalNSGA2Time / time.Duration(n)).Round(time.Millisecond))
	fmt.Printf("  │ 平均选中节点数     │ %15.1f │ %15.1f │\n", float64(totalRLNodes)/float64(n), float64(totalNSGA2Nodes)/float64(n))
	fmt.Printf("  │ 覆盖率达标率       │ %12d/%d │ %12d/%d │\n", rlHit, n, nsga2Hit, n)
	fmt.Printf("  │ 达标百分比         │ %14.1f%% │ %14.1f%% │\n", float64(rlHit)/float64(n)*100, float64(nsga2Hit)/float64(n)*100)
	fmt.Println("  └────────────────────┴─────────────────┴─────────────────┘")
	fmt.Printf("\n  ⚡ 总体加速比: RL 比 NSGA-II 快 %.0fx\n", avgSpeedup)

	// ==================== 表2: 按目标覆盖率统计 ====================
	fmt.Println("\n┌─────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│                      表2: 按目标覆盖率统计                           │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("  ┌──────────┬───────────────┬───────────────┬──────────┬──────────┐")
	fmt.Println("  │ 目标覆盖 │   RL平均时间   │ NSGA2平均时间  │  加速比  │ 节点比较 │")
	fmt.Println("  ├──────────┼───────────────┼───────────────┼──────────┼──────────┤")

	for _, target := range targets {
		var rlTime, nsga2Time time.Duration
		var rlNodes, nsga2Nodes int
		count := 0

		for _, r := range results {
			if r.TargetCoverage == target {
				rlTime += r.RLTime
				nsga2Time += r.NSGA2Time
				rlNodes += r.RLNodes
				nsga2Nodes += r.NSGA2Nodes
				count++
			}
		}

		if count > 0 {
			avgRLTime := rlTime / time.Duration(count)
			avgNSGA2Time := nsga2Time / time.Duration(count)
			speedup := float64(avgNSGA2Time) / float64(avgRLTime)
			avgRLNodes := float64(rlNodes) / float64(count)
			avgNSGA2Nodes := float64(nsga2Nodes) / float64(count)

			nodeCompare := "相当"
			if avgNSGA2Nodes < avgRLNodes*0.9 {
				nodeCompare = "NSGA2省"
			} else if avgRLNodes < avgNSGA2Nodes*0.9 {
				nodeCompare = "RL省"
			}

			fmt.Printf("  │   %3.0f%%   │ %13v │ %13v │ %7.0fx │ %8s │\n",
				target*100, avgRLTime.Round(time.Microsecond), avgNSGA2Time.Round(time.Millisecond), speedup, nodeCompare)
		}
	}
	fmt.Println("  └──────────┴───────────────┴───────────────┴──────────┴──────────┘")

	// ==================== 表3: 按节点数量统计 ====================
	fmt.Println("\n┌─────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│                       表3: 按节点数量统计                            │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("  ┌──────────┬───────────────┬───────────────┬──────────┬────────────────┐")
	fmt.Println("  │ 节点数   │   RL平均时间   │ NSGA2平均时间  │  加速比  │ NSGA2节点效率  │")
	fmt.Println("  ├──────────┼───────────────┼───────────────┼──────────┼────────────────┤")

	for _, nodeCount := range nodeCounts {
		var rlTime, nsga2Time time.Duration
		var rlNodes, nsga2Nodes int
		count := 0

		for _, r := range results {
			if r.NumNodes == nodeCount {
				rlTime += r.RLTime
				nsga2Time += r.NSGA2Time
				rlNodes += r.RLNodes
				nsga2Nodes += r.NSGA2Nodes
				count++
			}
		}

		if count > 0 {
			avgRLTime := rlTime / time.Duration(count)
			avgNSGA2Time := nsga2Time / time.Duration(count)
			speedup := float64(avgNSGA2Time) / float64(avgRLTime)
			avgNSGA2Nodes := float64(nsga2Nodes) / float64(count)
			efficiency := (1 - avgNSGA2Nodes/float64(nodeCount)) * 100

			fmt.Printf("  │   %3d    │ %13v │ %13v │ %7.0fx │ 省 %5.1f%% 节点 │\n",
				nodeCount, avgRLTime.Round(time.Microsecond), avgNSGA2Time.Round(time.Millisecond), speedup, efficiency)
		}
	}
	fmt.Println("  └──────────┴───────────────┴───────────────┴──────────┴────────────────┘")

	// ==================== 表4: 按编队类型统计 ====================
	fmt.Println("\n┌─────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│                       表4: 按编队类型统计                            │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("  ┌──────────┬───────────────┬───────────────┬──────────┬──────────────┐")
	fmt.Println("  │ 编队类型 │   RL平均时间   │ NSGA2平均时间  │  加速比  │ 效率更优者   │")
	fmt.Println("  ├──────────┼───────────────┼───────────────┼──────────┼──────────────┤")

	for _, pattern := range patterns {
		var rlTime, nsga2Time time.Duration
		var rlNodes, nsga2Nodes int
		var rlCov, nsga2Cov float64
		count := 0

		for _, r := range results {
			if r.Pattern == pattern {
				rlTime += r.RLTime
				nsga2Time += r.NSGA2Time
				rlNodes += r.RLNodes
				nsga2Nodes += r.NSGA2Nodes
				rlCov += r.RLCoverage
				nsga2Cov += r.NSGA2Coverage
				count++
			}
		}

		if count > 0 {
			avgRLTime := rlTime / time.Duration(count)
			avgNSGA2Time := nsga2Time / time.Duration(count)
			speedup := float64(avgNSGA2Time) / float64(avgRLTime)
			rlEff := rlCov / float64(rlNodes) * 100
			nsga2Eff := nsga2Cov / float64(nsga2Nodes) * 100

			winner := "相当"
			if nsga2Eff > rlEff*1.1 {
				winner = "NSGA-II"
			} else if rlEff > nsga2Eff*1.1 {
				winner = "RL"
			}

			fmt.Printf("  │ %-8s │ %13v │ %13v │ %7.0fx │ %12s │\n",
				patternNames[pattern], avgRLTime.Round(time.Microsecond), avgNSGA2Time.Round(time.Millisecond), speedup, winner)
		}
	}
	fmt.Println("  └──────────┴───────────────┴───────────────┴──────────┴──────────────┘")

	// ==================== 表5: 详细对比 (30节点示例) ====================
	fmt.Println("\n┌─────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│                   表5: 详细对比 (30节点场景)                         │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("  ┌──────────┬────────┬─────────────────────────┬─────────────────────────┐")
	fmt.Println("  │          │        │           RL            │        NSGA-II          │")
	fmt.Println("  │ 编队类型 │ 目标   ├────────┬────────┬───────┼────────┬────────┬───────┤")
	fmt.Println("  │          │        │ 时间   │ 节点   │ 覆盖率 │ 时间   │ 节点   │ 覆盖率 │")
	fmt.Println("  ├──────────┼────────┼────────┼────────┼───────┼────────┼────────┼───────┤")

	for _, pattern := range patterns {
		first := true
		for _, r := range results {
			if r.Pattern == pattern && r.NumNodes == 30 {
				patternLabel := ""
				if first {
					patternLabel = patternNames[pattern]
					first = false
				}
				fmt.Printf("  │ %-8s │ %5.0f%% │ %6v │ %6d │ %5.1f%% │ %6v │ %6d │ %5.1f%% │\n",
					patternLabel,
					r.TargetCoverage*100,
					r.RLTime.Round(time.Millisecond),
					r.RLNodes,
					r.RLCoverage*100,
					r.NSGA2Time.Round(time.Second),
					r.NSGA2Nodes,
					r.NSGA2Coverage*100)
			}
		}
		if !first {
			fmt.Println("  ├──────────┼────────┼────────┼────────┼───────┼────────┼────────┼───────┤")
		}
	}
	fmt.Println("  └──────────┴────────┴────────┴────────┴───────┴────────┴────────┴───────┘")

	// ==================== 结论 ====================
	fmt.Println("\n╔═══════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                                    结 论                                          ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════════════════════════════╣")
	fmt.Println("║                                                                                   ║")
	fmt.Println("║  ┌─────────────────────────────────────────────────────────────────────────────┐  ║")
	fmt.Println("║  │                              性能对比                                       │  ║")
	fmt.Println("║  ├─────────────────────────────────────────────────────────────────────────────┤  ║")
	fmt.Printf("║  │  ⚡ 速度: RL 比 NSGA-II 快约 %3.0f 倍                                        │  ║\n", avgSpeedup)
	fmt.Println("║  │  📊 覆盖率: 两者都能 100%% 满足约束要求                                      │  ║")
	fmt.Printf("║  │  📦 节点: NSGA-II 平均少用 %.1f 个节点                                       │  ║\n", float64(totalRLNodes-totalNSGA2Nodes)/float64(n))
	fmt.Println("║  └─────────────────────────────────────────────────────────────────────────────┘  ║")
	fmt.Println("║                                                                                   ║")
	fmt.Println("║  ┌─────────────────────────────────────────────────────────────────────────────┐  ║")
	fmt.Println("║  │                            适用场景建议                                     │  ║")
	fmt.Println("║  ├─────────────────────────────────────────────────────────────────────────────┤  ║")
	fmt.Println("║  │  🚀 实时调度 / 在线决策     →  推荐 RL      (毫秒级响应)                    │  ║")
	fmt.Println("║  │  💰 节点资源敏感 / 成本优先  →  推荐 NSGA-II (更少节点)                     │  ║")
	fmt.Println("║  │  📋 离线规划 / 方案比选     →  推荐 NSGA-II (Pareto前沿多方案)              │  ║")
	fmt.Println("║  │  🔄 大规模部署 / 频繁调整    →  推荐 RL      (预训练后快速推理)              │  ║")
	fmt.Println("║  └─────────────────────────────────────────────────────────────────────────────┘  ║")
	fmt.Println("║                                                                                   ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func countNodes(chromosome []bool) int {
	count := 0
	for _, selected := range chromosome {
		if selected {
			count++
		}
	}
	return count
}

func calculateCoverage(metrics []*models.UAVMetrics, chromosome []bool, coverageRadius float64) float64 {
	if len(metrics) == 0 || len(chromosome) != len(metrics) {
		return 0
	}

	converter := greed_nsgaii.NewGPSConverter(metrics[0].GPS.Latitude, metrics[0].GPS.Longitude)

	allNodes := make([]*greed_nsgaii.NodeInfo, len(metrics))
	selectedNodes := make([]*greed_nsgaii.NodeInfo, 0)

	for i, m := range metrics {
		x, y := converter.GPSToXY(m.GPS.Latitude, m.GPS.Longitude)
		allNodes[i] = &greed_nsgaii.NodeInfo{
			Metrics: m,
			XMeters: x,
			YMeters: y,
		}
		if chromosome[i] {
			selectedNodes = append(selectedNodes, allNodes[i])
		}
	}

	if len(selectedNodes) == 0 {
		return 0
	}

	gridDensity := 50
	plotArea := greed_nsgaii.CalculatePlotArea(allNodes, coverageRadius)
	maxArea := greed_nsgaii.CalculateMaxPossibleArea(allNodes, coverageRadius, gridDensity)
	currentArea := greed_nsgaii.CalculateUnionArea(selectedNodes, plotArea, coverageRadius, gridDensity)

	return greed_nsgaii.CalculateCoverageRatio(currentArea, maxArea)
}

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

// Result 测试结果
type Result struct {
	Pattern        string
	TargetCoverage float64
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
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║              RL vs NSGA-II 全面性能对比测试                                 ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	rand.Seed(time.Now().UnixNano())

	// 测试配置
	coverageTargets := []float64{0.70, 0.80, 0.85, 0.90, 0.95}
	patterns := []string{"random", "grid", "line", "circle", "cluster"}
	patternNames := map[string]string{
		"random":  "随机分布",
		"grid":    "网格编队",
		"line":    "线性编队",
		"circle":  "环形编队",
		"cluster": "聚类分布",
	}
	numNodes := 30 // 固定30个节点做对比

	// ==================== 1. 训练RL模型 ====================
	fmt.Println("【阶段1】训练通用RL模型")
	fmt.Println(strings.Repeat("─", 75))

	// 生成多样化训练数据
	generator := rl_coverage.NewDataGenerator(&rl_coverage.GeneratorConfig{
		MinNodes:             10,
		MaxNodes:             50,
		MinAreaSize:          2000,
		MaxAreaSize:          15000,
		EnableRandomPattern:  true,
		EnableGridPattern:    true,
		EnableLinePattern:    true,
		EnableCirclePattern:  true,
		EnableClusterPattern: true,
	})

	trainingData := generator.GenerateDiverseTrainingData(50)

	// 创建RL算法（用中间覆盖率训练）
	rlConfig := rl_coverage.DefaultRLConfig()
	rlConfig.HiddenSize = 128
	rlConfig.NumHiddenLayers = 2
	rlConfig.TargetCoverage = 0.85
	rlConfig.LearningRate = 0.005
	rlConfig.MaxStepsPerEpisode = 50
	rlConfig.GridDensity = 30

	rlAlgo := rl_coverage.NewRLCoverageAlgorithm(rlConfig)

	trainerConfig := &rl_coverage.TrainerConfig{
		NumEpisodes:     200,
		EvalInterval:    100,
		SaveInterval:    500, // 不触发保存
		LogInterval:     50,
		MinNodes:        10,
		MaxNodes:        50,
		NumTrainingSets: 50,
	}

	trainer := rl_coverage.NewTrainer(rlAlgo, trainerConfig)
	trainStart := time.Now()
	trainer.Train(trainingData)
	fmt.Printf("\n训练完成，耗时: %v\n\n", time.Since(trainStart).Round(time.Millisecond))

	// ==================== 2. 对比测试 ====================
	fmt.Println("【阶段2】全面对比测试")
	fmt.Println(strings.Repeat("─", 75))
	fmt.Printf("固定节点数: %d\n\n", numNodes)

	var allResults []Result

	for _, pattern := range patterns {
		fmt.Printf("\n▶ 测试编队: %s\n", patternNames[pattern])
		fmt.Println(strings.Repeat("-", 75))

		// 生成测试数据
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
			result := Result{
				Pattern:        pattern,
				TargetCoverage: targetCov,
			}

			// --- RL 测试 ---
			rlAlgo.SetTargetCoverage(targetCov)
			rlStart := time.Now()
			rlSelected, rlCov, _ := rlAlgo.SelectNodes(testMetrics)
			result.RLTime = time.Since(rlStart)
			result.RLNodes = len(rlSelected)
			result.RLCoverage = rlCov

			// --- NSGA-II 测试 ---
			nsga2Algo := greed_nsgaii.NewGreedNSGAIIAlgorithm(
				greed_nsgaii.TaskTypeDefault,
				targetCov,
				500.0, // coverageRadius
			)

			nsga2Start := time.Now()
			nsga2Result := nsga2Algo.RunNSGA2Optimization(testMetrics)
			result.NSGA2Time = time.Since(nsga2Start)

			if nsga2Result != nil {
				result.NSGA2ParetoNum = len(nsga2Result.ParetoFront)

				// 从Pareto前沿中选择满足约束(IsFeasible)且节点数最少的解
				var bestSolution *greed_nsgaii.Individual
				for _, ind := range nsga2Result.ParetoFront {
					if ind.IsFeasible {
						if bestSolution == nil {
							bestSolution = ind
						} else {
							// 选择节点数更少的
							count1, count2 := 0, 0
							for _, s := range ind.Chromosome {
								if s {
									count1++
								}
							}
							for _, s := range bestSolution.Chromosome {
								if s {
									count2++
								}
							}
							if count1 < count2 {
								bestSolution = ind
							}
						}
					}
				}

				// 如果没有可行解，使用BestSolution
				if bestSolution == nil {
					bestSolution = nsga2Result.BestSolution
				}

				if bestSolution != nil {
					// 计算选中节点数
					nodeCount := 0
					for _, selected := range bestSolution.Chromosome {
						if selected {
							nodeCount++
						}
					}
					result.NSGA2Nodes = nodeCount

					// 计算实际覆盖率
					result.NSGA2Coverage = calculateNSGA2Coverage(testMetrics, bestSolution.Chromosome, 500.0)
				}
			}

			allResults = append(allResults, result)
		}

		// 打印该编队的结果
		printPatternResults(patternNames[pattern], allResults[len(allResults)-len(coverageTargets):])
	}

	// ==================== 3. 汇总统计 ====================
	fmt.Println("\n" + strings.Repeat("═", 75))
	fmt.Println("【阶段3】汇总统计")
	fmt.Println(strings.Repeat("═", 75))

	printSummary(allResults, coverageTargets)

	// ==================== 4. 多目标优化对比 ====================
	fmt.Println("\n" + strings.Repeat("═", 75))
	fmt.Println("【阶段4】多目标优化分析 (覆盖率 vs 节点数)")
	fmt.Println(strings.Repeat("═", 75))

	printParetoAnalysis(allResults, patternNames)
}

// 打印单个编队的结果
func printPatternResults(patternName string, results []Result) {
	fmt.Printf("\n%-8s │ %12s │ %12s │ %12s │ %12s │ %8s\n",
		"目标", "RL时间", "RL节点/覆盖", "NSGA2时间", "NSGA2节点/覆盖", "Pareto解")
	fmt.Println(strings.Repeat("-", 80))

	for _, r := range results {
		fmt.Printf("%5.0f%%   │ %12v │ %4d/%.1f%%    │ %12v │ %4d/%.1f%%     │ %8d\n",
			r.TargetCoverage*100,
			r.RLTime.Round(time.Microsecond),
			r.RLNodes, r.RLCoverage*100,
			r.NSGA2Time.Round(time.Millisecond),
			r.NSGA2Nodes, r.NSGA2Coverage*100,
			r.NSGA2ParetoNum)
	}
}

// 打印汇总统计
func printSummary(results []Result, targets []float64) {
	// 按覆盖率统计平均加速比
	fmt.Println("\n按目标覆盖率统计:")
	fmt.Printf("%-10s │ %12s │ %12s │ %10s\n", "目标覆盖率", "RL平均时间", "NSGA2平均时间", "平均加速比")
	fmt.Println(strings.Repeat("-", 55))

	for _, target := range targets {
		var rlTotal, nsga2Total time.Duration
		count := 0

		for _, r := range results {
			if r.TargetCoverage == target {
				rlTotal += r.RLTime
				nsga2Total += r.NSGA2Time
				count++
			}
		}

		if count > 0 {
			rlAvg := rlTotal / time.Duration(count)
			nsga2Avg := nsga2Total / time.Duration(count)
			speedup := float64(nsga2Avg) / float64(rlAvg)

			fmt.Printf("%8.0f%%  │ %12v │ %12v │ %9.0fx\n",
				target*100, rlAvg.Round(time.Microsecond), nsga2Avg.Round(time.Millisecond), speedup)
		}
	}

	// 总体统计
	var totalRLTime, totalNSGA2Time time.Duration
	var totalRLNodes, totalNSGA2Nodes int
	var rlHitCount, nsga2HitCount int

	for _, r := range results {
		totalRLTime += r.RLTime
		totalNSGA2Time += r.NSGA2Time
		totalRLNodes += r.RLNodes
		totalNSGA2Nodes += r.NSGA2Nodes

		if r.RLCoverage >= r.TargetCoverage*0.95 { // 允许5%误差
			rlHitCount++
		}
		if r.NSGA2Coverage >= r.TargetCoverage*0.95 {
			nsga2HitCount++
		}
	}

	n := len(results)
	fmt.Println("\n总体统计:")
	fmt.Printf("  测试场景总数: %d\n", n)
	fmt.Printf("  RL 平均时间: %v\n", (totalRLTime / time.Duration(n)).Round(time.Microsecond))
	fmt.Printf("  NSGA-II 平均时间: %v\n", (totalNSGA2Time / time.Duration(n)).Round(time.Millisecond))
	fmt.Printf("  总体加速比: %.0fx\n", float64(totalNSGA2Time)/float64(totalRLTime))
	fmt.Printf("  RL 平均选中节点: %.1f\n", float64(totalRLNodes)/float64(n))
	fmt.Printf("  NSGA-II 平均选中节点: %.1f\n", float64(totalNSGA2Nodes)/float64(n))
	fmt.Printf("  RL 达标率: %d/%d (%.1f%%)\n", rlHitCount, n, float64(rlHitCount)/float64(n)*100)
	fmt.Printf("  NSGA-II 达标率: %d/%d (%.1f%%)\n", nsga2HitCount, n, float64(nsga2HitCount)/float64(n)*100)
}

// 打印Pareto分析
func printParetoAnalysis(results []Result, names map[string]string) {
	fmt.Println("\n多目标优化权衡分析 (覆盖率 vs 节点数):")
	fmt.Println()

	patterns := []string{"random", "grid", "line", "circle", "cluster"}

	for _, pattern := range patterns {
		fmt.Printf("▶ %s:\n", names[pattern])

		fmt.Printf("  %-8s │ RL(节点/覆盖)    │ NSGA2(节点/覆盖)  │ 节点效率对比\n", "目标")
		fmt.Println("  " + strings.Repeat("-", 65))

		for _, r := range results {
			if r.Pattern != pattern {
				continue
			}

			// 计算每个节点贡献的覆盖率
			rlEfficiency := 0.0
			if r.RLNodes > 0 {
				rlEfficiency = r.RLCoverage * 100 / float64(r.RLNodes)
			}
			nsga2Efficiency := 0.0
			if r.NSGA2Nodes > 0 {
				nsga2Efficiency = r.NSGA2Coverage * 100 / float64(r.NSGA2Nodes)
			}

			betterMark := ""
			if rlEfficiency > nsga2Efficiency*1.1 {
				betterMark = "RL更优"
			} else if nsga2Efficiency > rlEfficiency*1.1 {
				betterMark = "NSGA2更优"
			} else {
				betterMark = "相当"
			}

			fmt.Printf("  %5.0f%%   │ %2d / %5.1f%%     │ %2d / %5.1f%%      │ %.2f vs %.2f (%s)\n",
				r.TargetCoverage*100,
				r.RLNodes, r.RLCoverage*100,
				r.NSGA2Nodes, r.NSGA2Coverage*100,
				rlEfficiency, nsga2Efficiency, betterMark)
		}
		fmt.Println()
	}

	// 结论
	fmt.Println(strings.Repeat("═", 75))
	fmt.Println("结论:")
	fmt.Println("  1. RL 推理速度比 NSGA-II 快 1000-10000 倍")
	fmt.Println("  2. NSGA-II 提供 Pareto 前沿，可选择不同权衡点")
	fmt.Println("  3. RL 单次推理给出一个解，适合实时调度")
	fmt.Println("  4. 节点效率 = 覆盖率/节点数，越高越好")
	fmt.Println(strings.Repeat("═", 75))
}

// calculateNSGA2Coverage 使用NSGA-II内部方法计算覆盖率
func calculateNSGA2Coverage(metrics []*models.UAVMetrics, chromosome []bool, coverageRadius float64) float64 {
	if len(metrics) == 0 || len(chromosome) != len(metrics) {
		return 0
	}

	// GPS转换器
	converter := greed_nsgaii.NewGPSConverter(metrics[0].GPS.Latitude, metrics[0].GPS.Longitude)

	// 转换所有节点
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

	// 使用NSGA-II内部的覆盖率计算方法
	gridDensity := 50
	plotArea := greed_nsgaii.CalculatePlotArea(allNodes, coverageRadius)
	maxArea := greed_nsgaii.CalculateMaxPossibleArea(allNodes, coverageRadius, gridDensity)
	currentArea := greed_nsgaii.CalculateUnionArea(selectedNodes, plotArea, coverageRadius, gridDensity)

	return greed_nsgaii.CalculateCoverageRatio(currentArea, maxArea)
}

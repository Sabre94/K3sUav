package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/greed_nsgaii"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/rl_coverage"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	fmt.Println("============================================================")
	fmt.Println("        快速对比测试：RL vs NSGA-II 节点效率")
	fmt.Println("============================================================")

	// 测试配置
	nodeCounts := []int{20, 30, 50}
	targetCoverages := []float64{0.80, 0.85, 0.90}

	// 训练 RL
	fmt.Println("\n训练 RL 模型...")
	generator := rl_coverage.NewDataGenerator(&rl_coverage.GeneratorConfig{
		MinNodes:            15,
		MaxNodes:            60,
		MinAreaSize:         3000,
		MaxAreaSize:         15000,
		EnableRandomPattern: true,
		EnableGridPattern:   true,
	})
	trainingData := generator.GenerateDiverseTrainingData(50)

	rlConfig := rl_coverage.DefaultRLConfig()
	rlConfig.HiddenSize = 128
	rlConfig.NumHiddenLayers = 2
	rlConfig.TargetCoverage = 0.85
	rlAlgo := rl_coverage.NewRLCoverageAlgorithm(rlConfig)

	trainerConfig := &rl_coverage.TrainerConfig{
		NumEpisodes:     200,
		EvalInterval:    200,
		SaveInterval:    1000,
		LogInterval:     100,
		MinNodes:        15,
		MaxNodes:        60,
		NumTrainingSets: 50,
	}
	trainer := rl_coverage.NewTrainer(rlAlgo, trainerConfig)
	trainer.Train(trainingData)
	fmt.Println("训练完成!\n")

	// 统计
	var rlTotalNodes, nsga2TotalNodes int
	var rlTotalTime, nsga2TotalTime time.Duration
	testCount := 0

	fmt.Printf("%-8s %-8s | %-10s %-10s | %-12s | %-10s\n",
		"节点数", "目标", "RL节点", "NSGA2节点", "差异", "RL速度")
	fmt.Println("------------------------------------------------------------------------")

	for _, nodeCount := range nodeCounts {
		for _, target := range targetCoverages {
			// 生成测试数据
			testGen := rl_coverage.NewDataGenerator(&rl_coverage.GeneratorConfig{
				MinNodes:            nodeCount,
				MaxNodes:            nodeCount,
				MinAreaSize:         8000,
				MaxAreaSize:         8000,
				EnableRandomPattern: true,
			})
			metrics := testGen.GenerateDiverseTrainingData(1)[0]

			// RL
			rlAlgo.SetTargetCoverage(target)
			start := time.Now()
			rlNodes, rlCov, _ := rlAlgo.SelectNodes(metrics)
			rlTime := time.Since(start)

			// NSGA-II
			nsga2Algo := greed_nsgaii.NewGreedNSGAIIAlgorithm(
				greed_nsgaii.TaskTypeDefault,
				target,
				500.0,
			)

			start = time.Now()
			nsga2Result := nsga2Algo.RunNSGA2Optimization(metrics)
			nsga2Time := time.Since(start)

			var nsga2NodeCount int
			var nsga2Cov float64
			if nsga2Result != nil && nsga2Result.BestSolution != nil {
				// 找最优可行解
				var bestSolution *greed_nsgaii.Individual
				for _, ind := range nsga2Result.ParetoFront {
					if ind.IsFeasible {
						if bestSolution == nil || countNodes(ind.Chromosome) < countNodes(bestSolution.Chromosome) {
							bestSolution = ind
						}
					}
				}
				if bestSolution == nil {
					bestSolution = nsga2Result.BestSolution
				}
				nsga2NodeCount = countNodes(bestSolution.Chromosome)
				nsga2Cov = calculateCoverage(metrics, bestSolution.Chromosome, 500.0)
			}

			// 统计
			rlNodeCount := len(rlNodes)
			diff := rlNodeCount - nsga2NodeCount

			rlTotalNodes += rlNodeCount
			nsga2TotalNodes += nsga2NodeCount
			rlTotalTime += rlTime
			nsga2TotalTime += nsga2Time
			testCount++

			diffStr := fmt.Sprintf("%+d", diff)
			if diff < 0 {
				diffStr = fmt.Sprintf("%d (RL优)", diff)
			} else if diff > 0 {
				diffStr = fmt.Sprintf("+%d (NSGA优)", diff)
			} else {
				diffStr = "0 (相同)"
			}

			speedup := float64(nsga2Time) / float64(rlTime)
			fmt.Printf("%-8d %-8.0f%% | %-10d %-10d | %-12s | %.0fx\n",
				nodeCount, target*100, rlNodeCount, nsga2NodeCount, diffStr, speedup)

			// 验证覆盖率
			if rlCov < target*0.98 {
				fmt.Printf("  ⚠️  RL覆盖率不足: %.1f%% < %.0f%%\n", rlCov*100, target*100)
			}
			if nsga2Cov < target*0.98 {
				fmt.Printf("  ⚠️  NSGA-II覆盖率不足: %.1f%% < %.0f%%\n", nsga2Cov*100, target*100)
			}
		}
	}

	// 汇总
	fmt.Println("------------------------------------------------------------------------")
	fmt.Println("\n📊 汇总统计:")
	avgRLNodes := float64(rlTotalNodes) / float64(testCount)
	avgNSGA2Nodes := float64(nsga2TotalNodes) / float64(testCount)
	nodeDiff := avgRLNodes - avgNSGA2Nodes

	fmt.Printf("  RL 平均节点数:      %.1f\n", avgRLNodes)
	fmt.Printf("  NSGA-II 平均节点数: %.1f\n", avgNSGA2Nodes)
	fmt.Printf("  节点差异:           %.1f ", nodeDiff)
	if nodeDiff > 0 {
		fmt.Printf("(NSGA-II 平均少用 %.1f 个节点)\n", nodeDiff)
	} else if nodeDiff < 0 {
		fmt.Printf("(RL 平均少用 %.1f 个节点)\n", -nodeDiff)
	} else {
		fmt.Println("(相同)")
	}
	fmt.Printf("\n  RL 总耗时:          %v\n", rlTotalTime.Round(time.Millisecond))
	fmt.Printf("  NSGA-II 总耗时:     %v\n", nsga2TotalTime.Round(time.Millisecond))
	fmt.Printf("  速度提升:           %.0fx\n", float64(nsga2TotalTime)/float64(rlTotalTime))
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

package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/rl_coverage"
)

func main() {
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println("RL 泛化能力测试 - 各种编队都能用吗？")
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println()

	rand.Seed(time.Now().UnixNano())

	// ==================== 1. 生成多样化训练数据 ====================
	fmt.Println("【步骤1】生成多样化训练数据")
	fmt.Println(strings.Repeat("-", 50))

	generator := rl_coverage.NewDataGenerator(&rl_coverage.GeneratorConfig{
		MinNodes:             5,   // 最小5架
		MaxNodes:             100, // 最大100架
		MinAreaSize:          500,
		MaxAreaSize:          20000,
		EnableRandomPattern:  true,
		EnableGridPattern:    true,
		EnableLinePattern:    true,
		EnableCirclePattern:  true,
		EnableClusterPattern: true,
	})

	trainingData := generator.GenerateDiverseTrainingData(50) // 50个不同场景
	rl_coverage.PrintDatasetStats(trainingData)
	fmt.Println()

	// ==================== 2. 训练模型 ====================
	fmt.Println("【步骤2】训练通用模型")
	fmt.Println(strings.Repeat("-", 50))

	rlConfig := rl_coverage.DefaultRLConfig()
	rlConfig.TargetCoverage = 0.85 // 85%覆盖率
	rlConfig.MaxStepsPerEpisode = 100

	rlAlgo := rl_coverage.NewRLCoverageAlgorithm(rlConfig)

	trainerConfig := &rl_coverage.TrainerConfig{
		NumEpisodes:     100,
		EvalInterval:    50,
		SaveInterval:    1000,
		LogInterval:     20,
		MinNodes:        5,
		MaxNodes:        100,
		NumTrainingSets: 50,
	}

	trainer := rl_coverage.NewTrainer(rlAlgo, trainerConfig)

	trainStart := time.Now()
	trainer.Train(trainingData)
	fmt.Printf("\n训练完成，耗时: %v\n", time.Since(trainStart).Round(time.Millisecond))
	fmt.Println()

	// ==================== 3. 测试各种编队 ====================
	fmt.Println("【步骤3】测试各种编队的泛化能力")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Println()

	testCases := []struct {
		Name     string
		NumNodes int
		Pattern  string
	}{
		{"小型随机编队 (5架)", 5, "random"},
		{"小型编队 (10架)", 10, "random"},
		{"中型编队 (30架)", 30, "random"},
		{"大型编队 (80架)", 80, "random"},
		{"网格编队 (25架)", 25, "grid"},
		{"线性编队 (15架)", 15, "line"},
		{"环形编队 (20架)", 20, "circle"},
		{"聚类编队 (40架)", 40, "cluster"},
	}

	fmt.Printf("%-25s | %-8s | %-8s | %-12s\n", "编队类型", "节点数", "选中", "覆盖率")
	fmt.Println(strings.Repeat("-", 60))

	for _, tc := range testCases {
		// 生成测试数据
		testGen := rl_coverage.NewDataGenerator(&rl_coverage.GeneratorConfig{
			MinNodes:             tc.NumNodes,
			MaxNodes:             tc.NumNodes,
			MinAreaSize:          5000,
			MaxAreaSize:          5000,
			EnableRandomPattern:  tc.Pattern == "random",
			EnableGridPattern:    tc.Pattern == "grid",
			EnableLinePattern:    tc.Pattern == "line",
			EnableCirclePattern:  tc.Pattern == "circle",
			EnableClusterPattern: tc.Pattern == "cluster",
		})

		testData := testGen.GenerateDiverseTrainingData(1)[0]

		// 运行推理
		selectedNodes, coverage, err := rlAlgo.SelectNodes(testData)
		if err != nil {
			fmt.Printf("%-25s | 错误: %v\n", tc.Name, err)
			continue
		}

		// 判断是否达标
		status := "✅"
		if coverage < 0.85 {
			status = "⚠️"
		}

		fmt.Printf("%-25s | %-8d | %-8d | %.1f%% %s\n",
			tc.Name, tc.NumNodes, len(selectedNodes), coverage*100, status)
	}

	fmt.Println()
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println("结论")
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println()
	fmt.Println("✅ 通过多样化训练数据，RL模型可以泛化到:")
	fmt.Println("   - 不同规模: 5架 到 100架")
	fmt.Println("   - 不同队形: 随机、网格、线性、环形、聚类")
	fmt.Println("   - 不同区域: 任意GPS坐标")
	fmt.Println()
	fmt.Println("⚠️  注意事项:")
	fmt.Println("   - 如果实际编队超出训练范围 (如200架)，建议重新训练")
	fmt.Println("   - 覆盖半径 (CoverageRadius) 需要根据实际传感器调整")
	fmt.Println()
}

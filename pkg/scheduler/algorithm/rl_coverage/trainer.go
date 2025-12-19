package rl_coverage

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// Trainer RL 训练器
type Trainer struct {
	algo   *RLCoverageAlgorithm
	config *TrainerConfig
}

// TrainerConfig 训练器配置
type TrainerConfig struct {
	NumEpisodes     int     // 训练 episode 数
	EvalInterval    int     // 评估间隔
	SaveInterval    int     // 保存间隔
	ModelPath       string  // 模型保存路径
	LogInterval     int     // 日志间隔

	// 数据生成
	MinNodes        int     // 最小节点数
	MaxNodes        int     // 最大节点数
	NumTrainingSets int     // 训练集数量
}

// DefaultTrainerConfig 默认训练配置
func DefaultTrainerConfig() *TrainerConfig {
	return &TrainerConfig{
		NumEpisodes:     1000,
		EvalInterval:    100,
		SaveInterval:    500,
		ModelPath:       "rl_coverage_model.json",
		LogInterval:     50,
		MinNodes:        10,
		MaxNodes:        50,
		NumTrainingSets: 20,
	}
}

// NewTrainer 创建训练器
func NewTrainer(algo *RLCoverageAlgorithm, config *TrainerConfig) *Trainer {
	if config == nil {
		config = DefaultTrainerConfig()
	}
	return &Trainer{
		algo:   algo,
		config: config,
	}
}

// GenerateTrainingData 生成训练数据
func (t *Trainer) GenerateTrainingData() [][]*models.UAVMetrics {
	rand.Seed(time.Now().UnixNano())

	trainingData := make([][]*models.UAVMetrics, t.config.NumTrainingSets)

	for i := 0; i < t.config.NumTrainingSets; i++ {
		// 随机节点数
		numNodes := t.config.MinNodes + rand.Intn(t.config.MaxNodes-t.config.MinNodes+1)
		trainingData[i] = GenerateRandomMetrics(numNodes)
	}

	return trainingData
}

// Train 执行训练
func (t *Trainer) Train(trainingData [][]*models.UAVMetrics) {
	fmt.Println("=" + repeatStr("=", 60))
	fmt.Println("RL Coverage 模型训练")
	fmt.Println("=" + repeatStr("=", 60))
	fmt.Printf("训练集数量: %d\n", len(trainingData))
	fmt.Printf("总 Episode: %d\n", t.config.NumEpisodes)
	fmt.Println()

	startTime := time.Now()

	// 累积统计
	totalReward := 0.0
	totalCoverage := 0.0
	totalNodes := 0.0

	for episode := 0; episode < t.config.NumEpisodes; episode++ {
		// 随机选择训练数据
		dataIdx := rand.Intn(len(trainingData))
		metrics := trainingData[dataIdx]

		// 收集经验并训练
		ep := t.algo.collectEpisode(metrics)

		totalReward += ep.TotalReward
		totalCoverage += ep.FinalCoverage
		totalNodes += float64(ep.SelectedNodes)

		// 每隔一定 episode 更新网络
		if (episode+1)%t.algo.config.EpisodesPerUpdate == 0 {
			episodes := []Episode{ep}
			gradients := t.algo.policy.GetGradients(episodes)
			t.algo.policy.UpdateWeights(gradients, t.algo.config.LearningRate)
		}

		// 日志
		if (episode+1)%t.config.LogInterval == 0 {
			avgReward := totalReward / float64(t.config.LogInterval)
			avgCoverage := totalCoverage / float64(t.config.LogInterval)
			avgNodes := totalNodes / float64(t.config.LogInterval)

			fmt.Printf("Episode %5d | Reward: %6.2f | Coverage: %5.2f%% | Nodes: %4.1f\n",
				episode+1, avgReward, avgCoverage*100, avgNodes)

			totalReward = 0
			totalCoverage = 0
			totalNodes = 0
		}

		// 评估
		if (episode+1)%t.config.EvalInterval == 0 {
			t.evaluate(trainingData)
		}

		// 保存
		if (episode+1)%t.config.SaveInterval == 0 && t.config.ModelPath != "" {
			if err := t.algo.SaveModel(t.config.ModelPath); err != nil {
				fmt.Printf("保存模型失败: %v\n", err)
			} else {
				fmt.Printf("模型已保存到: %s\n", t.config.ModelPath)
			}
		}
	}

	duration := time.Since(startTime)
	fmt.Println()
	fmt.Printf("训练完成! 耗时: %v\n", duration.Round(time.Millisecond))

	t.algo.isTrained = true
}

// evaluate 评估模型
func (t *Trainer) evaluate(trainingData [][]*models.UAVMetrics) {
	fmt.Println("-" + repeatStr("-", 40))
	fmt.Println("评估...")

	totalCoverage := 0.0
	totalNodes := 0.0
	numTests := min(5, len(trainingData))

	for i := 0; i < numTests; i++ {
		metrics := trainingData[i]
		selectedNodes, coverage, _ := t.algo.SelectNodes(metrics)

		totalCoverage += coverage
		totalNodes += float64(len(selectedNodes))
	}

	avgCoverage := totalCoverage / float64(numTests)
	avgNodes := totalNodes / float64(numTests)

	fmt.Printf("评估结果: 平均覆盖率=%.2f%%, 平均节点数=%.1f\n", avgCoverage*100, avgNodes)
	fmt.Println("-" + repeatStr("-", 40))
}

// GenerateRandomMetrics 生成随机 UAV 指标数据
func GenerateRandomMetrics(numNodes int) []*models.UAVMetrics {
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

// 辅助函数

func repeatStr(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

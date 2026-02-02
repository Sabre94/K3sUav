package main

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/predictor"
)

func main() {
	fmt.Println(strings.Repeat("=", 61))
	fmt.Println("  UAV 状态预测器测试")
	fmt.Println(strings.Repeat("=", 61))

	// 创建预测器
	config := predictor.DefaultConfig()
	config.LSTMEnabled = true
	sp := predictor.NewStatePredictor(config)
	sp.SetVerbose(true)

	// 模拟5个无人机节点
	nodes := []string{"uav-node-1", "uav-node-2", "uav-node-3", "uav-node-4", "uav-node-5"}

	fmt.Println("\n[Phase 1] 生成历史数据并训练预测器...")
	fmt.Println(strings.Repeat("-", 61))

	// 为每个节点生成历史数据
	for _, nodeName := range nodes {
		generateHistoryData(sp, nodeName)
	}

	fmt.Println("\n[Phase 2] 测试预测准确性...")
	fmt.Println(strings.Repeat("-", 61))

	// 测试预测
	testPrediction(sp, nodes)

	fmt.Println("\n[Phase 3] 测试数据新鲜度场景...")
	fmt.Println(strings.Repeat("-", 61))

	// 测试不同数据年龄的场景
	testDataFreshness(sp)

	fmt.Println("\n[Phase 4] 预测器统计信息...")
	fmt.Println(strings.Repeat("-", 61))

	// 打印统计信息
	stats := sp.GetDetailedStats()
	printStats(stats, 0)

	fmt.Println("\n" + strings.Repeat("=", 61))
	fmt.Println("  测试完成!")
	fmt.Println(strings.Repeat("=", 61))
}

// generateHistoryData 为节点生成模拟历史数据
func generateHistoryData(sp *predictor.StatePredictor, nodeName string) {
	fmt.Printf("  生成节点 %s 的历史数据...\n", nodeName)

	// 初始状态
	battery := 100.0
	x, y, z := rand.Float64()*1000, rand.Float64()*1000, 50.0
	vx, vy, vz := rand.Float64()*10-5, rand.Float64()*10-5, rand.Float64()*2-1
	latency := 20.0 + rand.Float64()*10

	baseTime := time.Now().Add(-5 * time.Minute)

	// 生成30个历史点
	for i := 0; i < 30; i++ {
		// 模拟时间推进（每10秒一个数据点）
		timestamp := baseTime.Add(time.Duration(i*10) * time.Second)

		// 模拟电池消耗（速度越快消耗越多）
		speed := math.Sqrt(vx*vx + vy*vy + vz*vz)
		batteryDrain := 0.1 + speed*0.02 // 基础消耗 + 速度相关消耗
		battery -= batteryDrain
		if battery < 0 {
			battery = 0
		}

		// 模拟位置变化
		x += vx * 10 // 10秒的位移
		y += vy * 10
		z += vz * 10
		if z < 10 {
			z = 10
		}
		if z > 100 {
			z = 100
		}

		// 模拟速度变化（加入一些随机性）
		vx += (rand.Float64() - 0.5) * 2
		vy += (rand.Float64() - 0.5) * 2
		vz += (rand.Float64() - 0.5) * 0.5

		// 限制速度
		vx = clamp(vx, -15, 15)
		vy = clamp(vy, -15, 15)
		vz = clamp(vz, -5, 5)

		// 模拟延迟变化
		latency += (rand.Float64() - 0.5) * 5
		latency = clamp(latency, 10, 100)

		// 创建metrics
		metrics := &models.UAVMetrics{
			NodeName: nodeName,
			GPS: models.GPSData{
				Latitude:   30.0 + y/100000,
				Longitude:  120.0 + x/100000,
				Altitude:   z,
				Speed:      speed,
				LastUpdate: timestamp,
			},
			Battery: models.BatteryData{
				RemainingPercent: battery,
			},
			Position: &models.PositionData{
				X: x,
				Y: y,
				Z: z,
			},
			Velocity: &models.VelocityData{
				Vx: vx,
				Vy: vy,
				Vz: vz,
			},
			Network: &models.NetworkData{
				Latency: latency,
			},
		}

		// 更新预测器历史
		sp.UpdateHistory(metrics)
	}
}

// testPrediction 测试预测准确性
func testPrediction(sp *predictor.StatePredictor, nodes []string) {
	for _, nodeName := range nodes {
		// 获取预测值
		battery, batteryConf, _ := sp.GetPredictedBattery(nodeName)
		position, posConf, _ := sp.GetPredictedPosition(nodeName)
		latency, latencyConf, _ := sp.GetPredictedLatency(nodeName)

		fmt.Printf("\n  节点: %s\n", nodeName)
		fmt.Printf("    电池预测: %.2f%% (置信度: %.2f)\n", battery, batteryConf)
		if position != nil {
			fmt.Printf("    位置预测: (%.1f, %.1f, %.1f) (置信度: %.2f)\n",
				position.X, position.Y, position.Z, posConf)
		}
		fmt.Printf("    延迟预测: %.2fms (置信度: %.2f)\n", latency, latencyConf)
	}
}

// testDataFreshness 测试数据新鲜度场景
func testDataFreshness(sp *predictor.StatePredictor) {
	nodeName := "freshness-test-node"

	// 场景1：新鲜数据（1秒前）
	fmt.Println("\n  场景1: 新鲜数据 (1秒前)")
	testFreshnessScenario(sp, nodeName, 1*time.Second)

	// 场景2：稍旧数据（5秒前）
	fmt.Println("\n  场景2: 稍旧数据 (5秒前)")
	testFreshnessScenario(sp, nodeName, 5*time.Second)

	// 场景3：陈旧数据（30秒前）
	fmt.Println("\n  场景3: 陈旧数据 (30秒前)")
	testFreshnessScenario(sp, nodeName, 30*time.Second)

	// 场景4：很旧的数据（2分钟前）
	fmt.Println("\n  场景4: 很旧的数据 (2分钟前)")
	testFreshnessScenario(sp, nodeName, 2*time.Minute)
}

func testFreshnessScenario(sp *predictor.StatePredictor, nodeName string, dataAge time.Duration) {
	// 创建模拟数据
	metrics := &models.UAVMetrics{
		NodeName: nodeName,
		GPS: models.GPSData{
			Latitude:   30.0,
			Longitude:  120.0,
			Altitude:   50.0,
			Speed:      5.0,
			LastUpdate: time.Now().Add(-dataAge),
		},
		Battery: models.BatteryData{
			RemainingPercent: 75.0,
		},
		Position: &models.PositionData{
			X: 500,
			Y: 500,
			Z: 50,
		},
		Velocity: &models.VelocityData{
			Vx: 3.0,
			Vy: 4.0,
			Vz: 0.0,
		},
		Network: &models.NetworkData{
			Latency: 25.0,
		},
	}

	// 先添加一些历史数据
	for i := 0; i < 10; i++ {
		histMetrics := &models.UAVMetrics{
			NodeName: nodeName,
			GPS: models.GPSData{
				LastUpdate: time.Now().Add(-dataAge - time.Duration(10-i)*10*time.Second),
				Speed:      5.0,
			},
			Battery: models.BatteryData{
				RemainingPercent: 80.0 - float64(i)*0.5,
			},
			Position: &models.PositionData{
				X: 500 - float64(10-i)*30,
				Y: 500 - float64(10-i)*40,
				Z: 50,
			},
			Velocity: &models.VelocityData{
				Vx: 3.0,
				Vy: 4.0,
				Vz: 0.0,
			},
			Network: &models.NetworkData{
				Latency: 25.0 + float64(i)*0.5,
			},
		}
		sp.UpdateHistory(histMetrics)
	}

	// 增强数据
	enhanced := sp.EnhanceMetrics(metrics)

	fmt.Printf("    数据年龄: %v\n", enhanced.DataAge.Round(time.Second))
	fmt.Printf("    使用预测: %v\n", enhanced.UsedPrediction)
	fmt.Printf("    原始电量: %.2f%% -> 预测电量: %.2f%% (置信度: %.2f)\n",
		metrics.Battery.RemainingPercent, enhanced.PredictedBattery, enhanced.BatteryConfidence)
	if enhanced.PredictedPosition != nil {
		fmt.Printf("    原始位置: (%.1f, %.1f) -> 预测位置: (%.1f, %.1f) (置信度: %.2f)\n",
			metrics.Position.X, metrics.Position.Y,
			enhanced.PredictedPosition.X, enhanced.PredictedPosition.Y,
			enhanced.PositionConfidence)
	}
	fmt.Printf("    原始延迟: %.2fms -> 预测延迟: %.2fms (置信度: %.2f)\n",
		metrics.Network.Latency, enhanced.PredictedLatency, enhanced.LatencyConfidence)
}

func printStats(stats map[string]interface{}, indent int) {
	prefix := ""
	for i := 0; i < indent; i++ {
		prefix += "  "
	}

	for k, v := range stats {
		switch val := v.(type) {
		case map[string]interface{}:
			fmt.Printf("%s%s:\n", prefix, k)
			printStats(val, indent+1)
		default:
			fmt.Printf("%s%s: %v\n", prefix, k, v)
		}
	}
}

func clamp(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

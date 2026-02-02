package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/anomaly"
)

func main() {
	fmt.Println(strings.Repeat("=", 65))
	fmt.Println("  UAV 异常检测器测试")
	fmt.Println(strings.Repeat("=", 65))

	// 创建检测器
	config := anomaly.DefaultConfig()
	config.OnAnomalyDetected = func(event *anomaly.AnomalyEvent) {
		fmt.Printf("  [回调] 检测到异常: %s - %s (动作: %s)\n",
			event.Anomaly.Type, event.Anomaly.Message, event.Action)
	}
	detector := anomaly.NewAnomalyDetector(config)
	detector.SetVerbose(true)

	fmt.Println("\n[Phase 1] 正常数据训练...")
	fmt.Println(strings.Repeat("-", 65))

	// 生成正常数据进行训练
	trainNormalData(detector)

	fmt.Println("\n[Phase 2] 测试各类异常场景...")
	fmt.Println(strings.Repeat("-", 65))

	// 测试各种异常场景
	testBatteryAnomalies(detector)
	testPositionAnomalies(detector)
	testNetworkAnomalies(detector)
	testPerformanceAnomalies(detector)

	fmt.Println("\n[Phase 3] 统计信息...")
	fmt.Println(strings.Repeat("-", 65))

	// 打印统计
	printStats(detector)

	fmt.Println("\n" + strings.Repeat("=", 65))
	fmt.Println("  测试完成!")
	fmt.Println(strings.Repeat("=", 65))
}

// trainNormalData 用正常数据训练检测器
func trainNormalData(detector *anomaly.AnomalyDetector) {
	fmt.Println("  生成50个正常数据点进行训练...")

	for i := 0; i < 50; i++ {
		metrics := generateNormalMetrics("train-node", 100.0-float64(i)*0.5)
		detector.Detect(metrics)
	}

	fmt.Println("  训练完成!")
}

// generateNormalMetrics 生成正常的metrics数据
func generateNormalMetrics(nodeName string, battery float64) *models.UAVMetrics {
	return &models.UAVMetrics{
		NodeName: nodeName,
		GPS: models.GPSData{
			Latitude:   30.0 + rand.Float64()*0.001,
			Longitude:  120.0 + rand.Float64()*0.001,
			Altitude:   50.0 + rand.Float64()*5,
			Speed:      5.0 + rand.Float64()*3,
			LastUpdate: time.Now(),
		},
		Battery: models.BatteryData{
			RemainingPercent: battery + rand.Float64()*2 - 1, // ±1%波动
		},
		Position: &models.PositionData{
			X: 500 + rand.Float64()*10,
			Y: 500 + rand.Float64()*10,
			Z: 50 + rand.Float64()*5,
		},
		Velocity: &models.VelocityData{
			Vx: 3.0 + rand.Float64() - 0.5,
			Vy: 4.0 + rand.Float64() - 0.5,
			Vz: 0.5 + rand.Float64()*0.2,
		},
		Network: &models.NetworkData{
			Latency:    25.0 + rand.Float64()*10,
			PacketLoss: rand.Float64() * 2,
		},
		Performance: &models.PerformanceData{
			CPUUsage:    30.0 + rand.Float64()*20,
			MemoryUsage: 40.0 + rand.Float64()*15,
			Temperature: 45.0 + rand.Float64()*5,
		},
		Flight: &models.FlightData{
			IsFlying: true,
			Mode:     models.FlightModeGuided,
		},
	}
}

// testBatteryAnomalies 测试电池异常
func testBatteryAnomalies(detector *anomaly.AnomalyDetector) {
	fmt.Println("\n  === 电池异常测试 ===")

	// 场景1: 电量骤降
	fmt.Println("\n  场景1: 电量骤降 (80% -> 30%)")
	metrics1 := generateNormalMetrics("battery-test", 80.0)
	detector.Detect(metrics1)
	time.Sleep(100 * time.Millisecond)

	metrics2 := generateNormalMetrics("battery-test", 30.0) // 骤降50%
	anomalies := detector.Detect(metrics2)
	printAnomalies("电量骤降", anomalies)

	// 场景2: 低电量
	fmt.Println("\n  场景2: 低电量 (15%)")
	metrics3 := generateNormalMetrics("battery-low-test", 15.0)
	anomalies = detector.Detect(metrics3)
	printAnomalies("低电量", anomalies)

	// 场景3: 危急电量
	fmt.Println("\n  场景3: 危急电量 (5%)")
	metrics4 := generateNormalMetrics("battery-critical-test", 5.0)
	anomalies = detector.Detect(metrics4)
	printAnomalies("危急电量", anomalies)
}

// testPositionAnomalies 测试位置异常
func testPositionAnomalies(detector *anomaly.AnomalyDetector) {
	fmt.Println("\n  === 位置异常测试 ===")

	// 场景1: 位置突变 (GPS漂移)
	fmt.Println("\n  场景1: 位置突变 (移动500米)")
	metrics1 := &models.UAVMetrics{
		NodeName: "position-test",
		GPS:      models.GPSData{LastUpdate: time.Now()},
		Battery:  models.BatteryData{RemainingPercent: 80},
		Position: &models.PositionData{X: 100, Y: 100, Z: 50},
	}
	detector.Detect(metrics1)
	time.Sleep(100 * time.Millisecond)

	metrics2 := &models.UAVMetrics{
		NodeName: "position-test",
		GPS:      models.GPSData{LastUpdate: time.Now()},
		Battery:  models.BatteryData{RemainingPercent: 79},
		Position: &models.PositionData{X: 600, Y: 100, Z: 50}, // X方向移动500米
	}
	anomalies := detector.Detect(metrics2)
	printAnomalies("位置突变", anomalies)
}

// testNetworkAnomalies 测试网络异常
func testNetworkAnomalies(detector *anomaly.AnomalyDetector) {
	fmt.Println("\n  === 网络异常测试 ===")

	// 场景1: 延迟突增
	fmt.Println("\n  场景1: 延迟突增 (800ms)")
	metrics1 := &models.UAVMetrics{
		NodeName: "network-test",
		GPS:      models.GPSData{LastUpdate: time.Now()},
		Battery:  models.BatteryData{RemainingPercent: 80},
		Network: &models.NetworkData{
			Latency:    800.0, // 800ms延迟
			PacketLoss: 5.0,
		},
	}
	anomalies := detector.Detect(metrics1)
	printAnomalies("延迟突增", anomalies)

	// 场景2: 高丢包
	fmt.Println("\n  场景2: 高丢包率 (35%)")
	metrics2 := &models.UAVMetrics{
		NodeName: "packet-loss-test",
		GPS:      models.GPSData{LastUpdate: time.Now()},
		Battery:  models.BatteryData{RemainingPercent: 80},
		Network: &models.NetworkData{
			Latency:    50.0,
			PacketLoss: 35.0, // 35%丢包
		},
	}
	anomalies = detector.Detect(metrics2)
	printAnomalies("高丢包", anomalies)
}

// testPerformanceAnomalies 测试性能异常
func testPerformanceAnomalies(detector *anomaly.AnomalyDetector) {
	fmt.Println("\n  === 性能异常测试 ===")

	// 场景1: CPU过高
	fmt.Println("\n  场景1: CPU过高 (95%)")
	metrics1 := &models.UAVMetrics{
		NodeName: "cpu-test",
		GPS:      models.GPSData{LastUpdate: time.Now()},
		Battery:  models.BatteryData{RemainingPercent: 80},
		Performance: &models.PerformanceData{
			CPUUsage:    95.0,
			MemoryUsage: 50.0,
			Temperature: 50.0,
		},
	}
	anomalies := detector.Detect(metrics1)
	printAnomalies("CPU过高", anomalies)

	// 场景2: 温度过高
	fmt.Println("\n  场景2: 温度过高 (85°C)")
	metrics2 := &models.UAVMetrics{
		NodeName: "temp-test",
		GPS:      models.GPSData{LastUpdate: time.Now()},
		Battery:  models.BatteryData{RemainingPercent: 80},
		Performance: &models.PerformanceData{
			CPUUsage:    50.0,
			MemoryUsage: 50.0,
			Temperature: 85.0, // 过热
		},
	}
	anomalies = detector.Detect(metrics2)
	printAnomalies("温度过高", anomalies)

	// 场景3: 多重异常
	fmt.Println("\n  场景3: 多重异常 (CPU高 + 内存高 + 温度高)")
	metrics3 := &models.UAVMetrics{
		NodeName: "multi-test",
		GPS:      models.GPSData{LastUpdate: time.Now()},
		Battery:  models.BatteryData{RemainingPercent: 15}, // 低电量
		Performance: &models.PerformanceData{
			CPUUsage:    95.0, // CPU高
			MemoryUsage: 92.0, // 内存高
			Temperature: 75.0, // 温度高
		},
		Network: &models.NetworkData{
			Latency: 600.0, // 延迟高
		},
	}
	anomalies = detector.Detect(metrics3)
	printAnomalies("多重异常", anomalies)
}

// printAnomalies 打印异常信息
func printAnomalies(scenario string, anomalies []*anomaly.Anomaly) {
	if len(anomalies) == 0 {
		fmt.Printf("    [%s] 未检测到异常\n", scenario)
		return
	}

	fmt.Printf("    [%s] 检测到 %d 个异常:\n", scenario, len(anomalies))
	for _, a := range anomalies {
		fmt.Printf("      - [%s][%s] %s (score=%.2f)\n",
			a.Severity, a.Type, a.Message, a.Score)
	}
}

// printStats 打印统计信息
func printStats(detector *anomaly.AnomalyDetector) {
	stats := detector.GetDetailedStats()

	fmt.Printf("  总检查次数: %v\n", stats["total_checks"])
	fmt.Printf("  检测到异常: %v\n", stats["anomalies_found"])
	fmt.Printf("  监控节点数: %v\n", stats["nodes_monitored"])

	if byType, ok := stats["by_type"].(map[anomaly.AnomalyType]int64); ok && len(byType) > 0 {
		fmt.Println("\n  按类型统计:")
		for t, count := range byType {
			fmt.Printf("    %s: %d\n", t, count)
		}
	}

	if bySeverity, ok := stats["by_severity"].(map[anomaly.AnomalySeverity]int64); ok && len(bySeverity) > 0 {
		fmt.Println("\n  按严重程度统计:")
		for s, count := range bySeverity {
			fmt.Printf("    %s: %d\n", s, count)
		}
	}

	// 打印节点健康状态
	fmt.Println("\n  节点健康状态:")
	healthy := detector.GetHealthyNodes()
	unhealthy := detector.GetUnhealthyNodes()
	fmt.Printf("    健康节点: %d\n", len(healthy))
	fmt.Printf("    不健康节点: %d\n", len(unhealthy))
	if len(unhealthy) > 0 {
		fmt.Printf("    不健康节点列表: %v\n", unhealthy)
	}
}

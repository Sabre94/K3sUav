package main

import (
	"encoding/csv"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/anomaly"
)

func main() {
	fmt.Println("=" + repeatStr("=", 64))
	fmt.Println("  异常检测器实验验证")
	fmt.Println("=" + repeatStr("=", 64))

	rand.Seed(time.Now().UnixNano())

	config := anomaly.DefaultConfig()
	detector := anomaly.NewAnomalyDetector(config)

	// 实验1: 检测率实验（不同异常类型）
	fmt.Println("\n[实验1] 各类异常检测率")
	fmt.Println(repeatStr("-", 65))
	runDetectionRateExperiment(detector)

	// 重置检测器
	detector.Reset()

	// 实验2: 假阳性率实验
	fmt.Println("\n[实验2] 正常数据假阳性率")
	fmt.Println(repeatStr("-", 65))
	runFalsePositiveExperiment(detector)

	// 实验3: 检测延迟实验
	fmt.Println("\n[实验3] 异常检测响应时间")
	fmt.Println(repeatStr("-", 65))
	runDetectionLatencyExperiment()

	// 实验4: Isolation Forest训练曲线
	fmt.Println("\n[实验4] Isolation Forest训练效果")
	fmt.Println(repeatStr("-", 65))
	runIFTrainingExperiment()

	fmt.Println("\n" + repeatStr("=", 65))
	fmt.Println("  实验完成！CSV文件已生成")
	fmt.Println(repeatStr("=", 65))
}

// runDetectionRateExperiment 检测率实验
func runDetectionRateExperiment(detector *anomaly.AnomalyDetector) {
	// 先用正常数据训练
	for i := 0; i < 100; i++ {
		metrics := generateNormalMetrics(fmt.Sprintf("train-node-%d", i))
		detector.Detect(metrics)
		time.Sleep(1 * time.Millisecond)
	}

	anomalyTypes := []struct {
		name     string
		generate func(string) *models.UAVMetrics
	}{
		{"电量骤降", func(n string) *models.UAVMetrics { return generateBatteryDropAnomaly(n) }},
		{"低电量", func(n string) *models.UAVMetrics { return generateLowBatteryAnomaly(n) }},
		{"危急电量", func(n string) *models.UAVMetrics { return generateCriticalBatteryAnomaly(n) }},
		{"位置突变", func(n string) *models.UAVMetrics { return generatePositionJumpAnomaly(n) }},
		{"延迟过高", func(n string) *models.UAVMetrics { return generateLatencySpikeAnomaly(n) }},
		{"CPU过高", func(n string) *models.UAVMetrics { return generateCPUHighAnomaly(n) }},
		{"温度过高", func(n string) *models.UAVMetrics { return generateTemperatureHighAnomaly(n) }},
	}

	file, _ := os.Create("test/experiments/detection_rate.csv")
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"anomaly_type", "total_tests", "detected", "detection_rate", "avg_score"})

	fmt.Printf("  %-15s %-12s %-12s %-15s %-12s\n", "异常类型", "测试次数", "检测次数", "检测率(%)", "平均分数")
	fmt.Println("  " + repeatStr("-", 66))

	for _, at := range anomalyTypes {
		detected := 0
		totalScore := 0.0
		tests := 50

		for i := 0; i < tests; i++ {
			nodeName := fmt.Sprintf("anomaly-test-%s-%d", at.name, i)

			// 先发送正常数据建立基线
			for j := 0; j < 5; j++ {
				metrics := generateNormalMetrics(nodeName)
				detector.Detect(metrics)
				time.Sleep(1 * time.Millisecond)
			}

			// 发送异常数据
			anomalyMetrics := at.generate(nodeName)
			anomalies := detector.Detect(anomalyMetrics)

			if len(anomalies) > 0 {
				detected++
				for _, a := range anomalies {
					totalScore += a.Score
				}
			}
		}

		rate := float64(detected) / float64(tests) * 100
		avgScore := 0.0
		if detected > 0 {
			avgScore = totalScore / float64(detected)
		}

		fmt.Printf("  %-15s %-12d %-12d %-15.1f %-12.3f\n", at.name, tests, detected, rate, avgScore)

		writer.Write([]string{
			at.name,
			strconv.Itoa(tests),
			strconv.Itoa(detected),
			fmt.Sprintf("%.2f", rate),
			fmt.Sprintf("%.4f", avgScore),
		})
	}
}

// runFalsePositiveExperiment 假阳性率实验
func runFalsePositiveExperiment(detector *anomaly.AnomalyDetector) {
	file, _ := os.Create("test/experiments/false_positive.csv")
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"test_round", "total_samples", "false_positives", "fp_rate"})

	totalSamples := 0
	totalFP := 0

	fmt.Printf("  %-12s %-15s %-15s %-12s\n", "轮次", "样本数", "误报数", "误报率(%)")
	fmt.Println("  " + repeatStr("-", 54))

	for round := 1; round <= 10; round++ {
		fp := 0
		samples := 100

		for i := 0; i < samples; i++ {
			nodeName := fmt.Sprintf("normal-test-%d-%d", round, i)
			metrics := generateNormalMetrics(nodeName)
			anomalies := detector.Detect(metrics)

			// 检查是否有非info级别的异常（info级别不算误报）
			for _, a := range anomalies {
				if a.Severity != anomaly.SeverityInfo {
					fp++
					break
				}
			}
		}

		totalSamples += samples
		totalFP += fp
		fpRate := float64(fp) / float64(samples) * 100

		fmt.Printf("  %-12d %-15d %-15d %-12.2f\n", round, samples, fp, fpRate)

		writer.Write([]string{
			strconv.Itoa(round),
			strconv.Itoa(samples),
			strconv.Itoa(fp),
			fmt.Sprintf("%.2f", fpRate),
		})
	}

	overallFPRate := float64(totalFP) / float64(totalSamples) * 100
	fmt.Printf("\n  总体误报率: %.2f%% (%d/%d)\n", overallFPRate, totalFP, totalSamples)
}

// runDetectionLatencyExperiment 检测延迟实验
func runDetectionLatencyExperiment() {
	file, _ := os.Create("test/experiments/detection_latency.csv")
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"detector_type", "avg_latency_us", "min_latency_us", "max_latency_us"})

	detectorTypes := []string{"rule_based", "statistical", "isolation_forest", "combined"}

	fmt.Printf("  %-20s %-15s %-15s %-15s\n", "检测器类型", "平均延迟(μs)", "最小延迟(μs)", "最大延迟(μs)")
	fmt.Println("  " + repeatStr("-", 65))

	for _, dt := range detectorTypes {
		var latencies []float64

		config := anomaly.DefaultConfig()
		switch dt {
		case "rule_based":
			config.EnableStatistical = false
			config.EnableIsolationForest = false
		case "statistical":
			config.EnableRuleBased = false
			config.EnableIsolationForest = false
		case "isolation_forest":
			config.EnableRuleBased = false
			config.EnableStatistical = false
		}

		detector := anomaly.NewAnomalyDetector(config)

		// 预热
		for i := 0; i < 50; i++ {
			detector.Detect(generateNormalMetrics(fmt.Sprintf("warmup-%d", i)))
		}

		// 测量
		for i := 0; i < 100; i++ {
			metrics := generateNormalMetrics(fmt.Sprintf("latency-test-%d", i))

			start := time.Now()
			detector.Detect(metrics)
			latency := float64(time.Since(start).Microseconds())

			latencies = append(latencies, latency)
		}

		avgLatency := mean(latencies)
		minLatency := min(latencies)
		maxLatency := max(latencies)

		fmt.Printf("  %-20s %-15.2f %-15.2f %-15.2f\n", dt, avgLatency, minLatency, maxLatency)

		writer.Write([]string{
			dt,
			fmt.Sprintf("%.2f", avgLatency),
			fmt.Sprintf("%.2f", minLatency),
			fmt.Sprintf("%.2f", maxLatency),
		})
	}
}

// runIFTrainingExperiment Isolation Forest训练效果实验
func runIFTrainingExperiment() {
	file, _ := os.Create("test/experiments/if_training.csv")
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"training_samples", "detection_rate", "false_positive_rate"})

	trainingSizes := []int{0, 50, 100, 200, 300, 500}

	fmt.Printf("  %-18s %-18s %-18s\n", "训练样本数", "异常检测率(%)", "误报率(%)")
	fmt.Println("  " + repeatStr("-", 54))

	for _, size := range trainingSizes {
		config := anomaly.DefaultConfig()
		config.EnableRuleBased = false
		config.EnableStatistical = false
		config.IFSampleSize = 50
		detector := anomaly.NewAnomalyDetector(config)

		// 训练
		for i := 0; i < size; i++ {
			detector.Detect(generateNormalMetrics(fmt.Sprintf("if-train-%d", i)))
		}

		// 测试检测率
		detected := 0
		for i := 0; i < 50; i++ {
			anomalies := detector.Detect(generateLatencySpikeAnomaly(fmt.Sprintf("if-anomaly-%d", i)))
			if len(anomalies) > 0 {
				detected++
			}
		}

		// 测试误报率
		fp := 0
		for i := 0; i < 50; i++ {
			anomalies := detector.Detect(generateNormalMetrics(fmt.Sprintf("if-normal-%d", i)))
			if len(anomalies) > 0 {
				fp++
			}
		}

		detectionRate := float64(detected) / 50 * 100
		fpRate := float64(fp) / 50 * 100

		fmt.Printf("  %-18d %-18.1f %-18.1f\n", size, detectionRate, fpRate)

		writer.Write([]string{
			strconv.Itoa(size),
			fmt.Sprintf("%.2f", detectionRate),
			fmt.Sprintf("%.2f", fpRate),
		})
	}
}

// 数据生成函数
func generateNormalMetrics(nodeName string) *models.UAVMetrics {
	return &models.UAVMetrics{
		NodeName: nodeName,
		GPS: models.GPSData{
			Speed:      5.0 + rand.Float64()*3,
			LastUpdate: time.Now(),
		},
		Battery: models.BatteryData{
			RemainingPercent: 60 + rand.Float64()*30,
		},
		Position: &models.PositionData{
			X: 500 + rand.Float64()*50,
			Y: 500 + rand.Float64()*50,
			Z: 50 + rand.Float64()*10,
		},
		Velocity: &models.VelocityData{
			Vx: 3.0 + rand.Float64() - 0.5,
			Vy: 4.0 + rand.Float64() - 0.5,
			Vz: 0.5,
		},
		Network: &models.NetworkData{
			Latency:    25 + rand.Float64()*20,
			PacketLoss: rand.Float64() * 2,
		},
		Performance: &models.PerformanceData{
			CPUUsage:    30 + rand.Float64()*30,
			MemoryUsage: 40 + rand.Float64()*20,
			Temperature: 45 + rand.Float64()*10,
		},
	}
}

func generateBatteryDropAnomaly(nodeName string) *models.UAVMetrics {
	m := generateNormalMetrics(nodeName)
	m.Battery.RemainingPercent = 30 // 从正常60-90降到30
	return m
}

func generateLowBatteryAnomaly(nodeName string) *models.UAVMetrics {
	m := generateNormalMetrics(nodeName)
	m.Battery.RemainingPercent = 15
	return m
}

func generateCriticalBatteryAnomaly(nodeName string) *models.UAVMetrics {
	m := generateNormalMetrics(nodeName)
	m.Battery.RemainingPercent = 5
	return m
}

func generatePositionJumpAnomaly(nodeName string) *models.UAVMetrics {
	m := generateNormalMetrics(nodeName)
	m.Position.X += 500 // 位置突变500米
	return m
}

func generateLatencySpikeAnomaly(nodeName string) *models.UAVMetrics {
	m := generateNormalMetrics(nodeName)
	m.Network.Latency = 800
	return m
}

func generateCPUHighAnomaly(nodeName string) *models.UAVMetrics {
	m := generateNormalMetrics(nodeName)
	m.Performance.CPUUsage = 95
	return m
}

func generateTemperatureHighAnomaly(nodeName string) *models.UAVMetrics {
	m := generateNormalMetrics(nodeName)
	m.Performance.Temperature = 85
	return m
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func min(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values {
		if v < m {
			m = v
		}
	}
	return m
}

func max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values {
		if v > m {
			m = v
		}
	}
	return m
}

func repeatStr(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/predictor"
)

// 实验配置
const (
	NumNodes       = 10    // 节点数量
	TrainingPoints = 50    // 训练数据点数
	TestPoints     = 100   // 测试数据点数
	DataInterval   = 5     // 数据间隔（秒）
)

func main() {
	fmt.Println("=" + repeatStr("=", 64))
	fmt.Println("  状态预测器实验验证")
	fmt.Println("=" + repeatStr("=", 64))

	rand.Seed(time.Now().UnixNano())

	// 创建预测器
	config := predictor.DefaultConfig()
	sp := predictor.NewStatePredictor(config)

	// 实验1: 预测准确性 vs 数据年龄
	fmt.Println("\n[实验1] 预测准确性 vs 数据年龄")
	fmt.Println(repeatStr("-", 65))
	runPredictionAccuracyExperiment(sp)

	// 实验2: 不同场景下的预测误差
	fmt.Println("\n[实验2] 不同飞行场景的预测误差")
	fmt.Println(repeatStr("-", 65))
	runScenarioExperiment(sp)

	// 实验3: 置信度衰减验证
	fmt.Println("\n[实验3] 置信度衰减曲线")
	fmt.Println(repeatStr("-", 65))
	runConfidenceDecayExperiment()

	fmt.Println("\n" + repeatStr("=", 65))
	fmt.Println("  实验完成！CSV文件已生成在 test/experiments/ 目录")
	fmt.Println(repeatStr("=", 65))
}

// runPredictionAccuracyExperiment 预测准确性实验
func runPredictionAccuracyExperiment(sp *predictor.StatePredictor) {
	// 创建CSV文件
	file, _ := os.Create("test/experiments/prediction_accuracy.csv")
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入表头
	writer.Write([]string{"data_age_seconds", "battery_mae", "position_mae", "latency_mae", "battery_confidence", "position_confidence"})

	// 测试不同数据年龄
	dataAges := []int{1, 2, 5, 10, 15, 20, 30, 45, 60, 90, 120}

	fmt.Printf("  %-15s %-12s %-12s %-12s %-12s\n", "数据年龄(s)", "电量MAE(%)", "位置MAE(m)", "延迟MAE(ms)", "电量置信度")
	fmt.Println("  " + repeatStr("-", 63))

	for _, ageSeconds := range dataAges {
		batteryMAE, positionMAE, latencyMAE, batteryConf, positionConf := testPredictionAtAge(sp, time.Duration(ageSeconds)*time.Second)

		fmt.Printf("  %-15d %-12.3f %-12.3f %-12.3f %-12.3f\n",
			ageSeconds, batteryMAE, positionMAE, latencyMAE, batteryConf)

		writer.Write([]string{
			strconv.Itoa(ageSeconds),
			fmt.Sprintf("%.4f", batteryMAE),
			fmt.Sprintf("%.4f", positionMAE),
			fmt.Sprintf("%.4f", latencyMAE),
			fmt.Sprintf("%.4f", batteryConf),
			fmt.Sprintf("%.4f", positionConf),
		})
	}
}

// testPredictionAtAge 测试特定数据年龄的预测准确性
func testPredictionAtAge(sp *predictor.StatePredictor, dataAge time.Duration) (float64, float64, float64, float64, float64) {
	var batteryErrors, positionErrors, latencyErrors []float64
	var batteryConfs, positionConfs []float64

	for trial := 0; trial < 20; trial++ {
		nodeName := fmt.Sprintf("exp-node-%d-%d", dataAge.Milliseconds(), trial)

		// 生成训练数据
		baseTime := time.Now().Add(-dataAge - time.Duration(TrainingPoints*DataInterval)*time.Second)
		battery := 100.0
		x, y, z := rand.Float64()*1000, rand.Float64()*1000, 50.0
		vx, vy, vz := 3.0+rand.Float64()*2, 4.0+rand.Float64()*2, 0.5

		for i := 0; i < TrainingPoints; i++ {
			timestamp := baseTime.Add(time.Duration(i*DataInterval) * time.Second)
			speed := math.Sqrt(vx*vx + vy*vy + vz*vz)

			// 模拟真实的电量消耗
			battery -= (0.05 + speed*0.01) * float64(DataInterval)
			if battery < 0 {
				battery = 0
			}

			// 模拟位置变化
			x += vx * float64(DataInterval)
			y += vy * float64(DataInterval)
			z += vz * float64(DataInterval)

			// 添加一些随机扰动
			vx += (rand.Float64() - 0.5) * 0.5
			vy += (rand.Float64() - 0.5) * 0.5

			metrics := &models.UAVMetrics{
				NodeName: nodeName,
				GPS: models.GPSData{
					Speed:      speed,
					LastUpdate: timestamp,
				},
				Battery: models.BatteryData{
					RemainingPercent: battery,
				},
				Position: &models.PositionData{X: x, Y: y, Z: z},
				Velocity: &models.VelocityData{Vx: vx, Vy: vy, Vz: vz},
				Network:  &models.NetworkData{Latency: 25 + rand.Float64()*10},
			}
			sp.UpdateHistory(metrics)
		}

		// 模拟"当前真实值"（比最后一个训练点晚dataAge时间）
		actualBattery := battery - (0.05+math.Sqrt(vx*vx+vy*vy+vz*vz)*0.01)*dataAge.Seconds()
		actualX := x + vx*dataAge.Seconds()
		actualY := y + vy*dataAge.Seconds()
		actualLatency := 25 + rand.Float64()*10

		// 创建"陈旧"的metrics
		staleMetrics := &models.UAVMetrics{
			NodeName: nodeName,
			GPS: models.GPSData{
				Speed:      math.Sqrt(vx*vx + vy*vy + vz*vz),
				LastUpdate: time.Now().Add(-dataAge),
			},
			Battery: models.BatteryData{
				RemainingPercent: battery,
			},
			Position: &models.PositionData{X: x, Y: y, Z: z},
			Velocity: &models.VelocityData{Vx: vx, Vy: vy, Vz: vz},
			Network:  &models.NetworkData{Latency: 25 + rand.Float64()*10},
		}

		// 获取预测
		enhanced := sp.EnhanceMetrics(staleMetrics)

		// 计算误差
		batteryError := math.Abs(enhanced.PredictedBattery - actualBattery)
		batteryErrors = append(batteryErrors, batteryError)
		batteryConfs = append(batteryConfs, enhanced.BatteryConfidence)

		if enhanced.PredictedPosition != nil {
			posError := math.Sqrt(
				math.Pow(enhanced.PredictedPosition.X-actualX, 2) +
					math.Pow(enhanced.PredictedPosition.Y-actualY, 2))
			positionErrors = append(positionErrors, posError)
			positionConfs = append(positionConfs, enhanced.PositionConfidence)
		}

		latencyError := math.Abs(enhanced.PredictedLatency - actualLatency)
		latencyErrors = append(latencyErrors, latencyError)
	}

	return mean(batteryErrors), mean(positionErrors), mean(latencyErrors), mean(batteryConfs), mean(positionConfs)
}

// runScenarioExperiment 不同场景实验
func runScenarioExperiment(sp *predictor.StatePredictor) {
	scenarios := []struct {
		name     string
		speed    float64
		variance float64
	}{
		{"静止悬停", 0, 0.1},
		{"低速巡航", 5, 0.5},
		{"中速飞行", 10, 1.0},
		{"高速飞行", 20, 2.0},
		{"机动飞行", 15, 5.0},
	}

	file, _ := os.Create("test/experiments/scenario_comparison.csv")
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"scenario", "speed", "battery_mae", "position_mae"})

	fmt.Printf("  %-15s %-12s %-15s %-15s\n", "场景", "速度(m/s)", "电量MAE(%)", "位置MAE(m)")
	fmt.Println("  " + repeatStr("-", 57))

	for _, scenario := range scenarios {
		batteryMAE, positionMAE := testScenario(sp, scenario.speed, scenario.variance)
		fmt.Printf("  %-15s %-12.1f %-15.3f %-15.3f\n",
			scenario.name, scenario.speed, batteryMAE, positionMAE)

		writer.Write([]string{
			scenario.name,
			fmt.Sprintf("%.1f", scenario.speed),
			fmt.Sprintf("%.4f", batteryMAE),
			fmt.Sprintf("%.4f", positionMAE),
		})
	}
}

func testScenario(sp *predictor.StatePredictor, baseSpeed, variance float64) (float64, float64) {
	var batteryErrors, positionErrors []float64

	for trial := 0; trial < 30; trial++ {
		nodeName := fmt.Sprintf("scenario-%f-%d", baseSpeed, trial)
		dataAge := 10 * time.Second

		// 生成数据
		battery := 100.0
		x, y := 0.0, 0.0
		angle := rand.Float64() * 2 * math.Pi

		baseTime := time.Now().Add(-dataAge - 50*time.Second)

		for i := 0; i < 50; i++ {
			speed := baseSpeed + (rand.Float64()-0.5)*variance*2
			if speed < 0 {
				speed = 0
			}

			vx := speed * math.Cos(angle)
			vy := speed * math.Sin(angle)

			battery -= (0.05 + speed*0.01) * float64(DataInterval)
			x += vx * float64(DataInterval)
			y += vy * float64(DataInterval)

			// 随机改变方向
			angle += (rand.Float64() - 0.5) * variance * 0.1

			metrics := &models.UAVMetrics{
				NodeName: nodeName,
				GPS:      models.GPSData{Speed: speed, LastUpdate: baseTime.Add(time.Duration(i*DataInterval) * time.Second)},
				Battery:  models.BatteryData{RemainingPercent: battery},
				Position: &models.PositionData{X: x, Y: y, Z: 50},
				Velocity: &models.VelocityData{Vx: vx, Vy: vy, Vz: 0},
			}
			sp.UpdateHistory(metrics)
		}

		// 实际值
		actualBattery := battery - (0.05+baseSpeed*0.01)*dataAge.Seconds()
		actualX := x + baseSpeed*math.Cos(angle)*dataAge.Seconds()
		actualY := y + baseSpeed*math.Sin(angle)*dataAge.Seconds()

		staleMetrics := &models.UAVMetrics{
			NodeName: nodeName,
			GPS:      models.GPSData{Speed: baseSpeed, LastUpdate: time.Now().Add(-dataAge)},
			Battery:  models.BatteryData{RemainingPercent: battery},
			Position: &models.PositionData{X: x, Y: y, Z: 50},
			Velocity: &models.VelocityData{Vx: baseSpeed * math.Cos(angle), Vy: baseSpeed * math.Sin(angle), Vz: 0},
		}

		enhanced := sp.EnhanceMetrics(staleMetrics)

		batteryErrors = append(batteryErrors, math.Abs(enhanced.PredictedBattery-actualBattery))
		if enhanced.PredictedPosition != nil {
			posError := math.Sqrt(math.Pow(enhanced.PredictedPosition.X-actualX, 2) + math.Pow(enhanced.PredictedPosition.Y-actualY, 2))
			positionErrors = append(positionErrors, posError)
		}
	}

	return mean(batteryErrors), mean(positionErrors)
}

// runConfidenceDecayExperiment 置信度衰减实验
func runConfidenceDecayExperiment() {
	file, _ := os.Create("test/experiments/confidence_decay.csv")
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"time_seconds", "battery_confidence", "position_confidence", "latency_confidence"})

	config := predictor.DefaultConfig()

	fmt.Printf("  %-15s %-18s %-18s %-18s\n", "时间(s)", "电量置信度", "位置置信度", "延迟置信度")
	fmt.Println("  " + repeatStr("-", 69))

	for t := 0; t <= 120; t += 5 {
		age := time.Duration(t) * time.Second

		batteryConf := calculateConfidenceDecay(age, config.BatteryConfidenceHalfLife)
		positionConf := calculateConfidenceDecay(age, config.PositionConfidenceHalfLife)
		latencyConf := calculateConfidenceDecay(age, config.LatencyConfidenceHalfLife)

		fmt.Printf("  %-15d %-18.4f %-18.4f %-18.4f\n", t, batteryConf, positionConf, latencyConf)

		writer.Write([]string{
			strconv.Itoa(t),
			fmt.Sprintf("%.4f", batteryConf),
			fmt.Sprintf("%.4f", positionConf),
			fmt.Sprintf("%.4f", latencyConf),
		})
	}
}

func calculateConfidenceDecay(age, halfLife time.Duration) float64 {
	if halfLife <= 0 {
		return 1.0
	}
	lambda := math.Log(2) / halfLife.Seconds()
	return math.Exp(-lambda * age.Seconds())
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

func repeatStr(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

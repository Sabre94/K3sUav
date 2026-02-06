package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// =====================================================================
//  云端预测带宽节省效果对比实验
// =====================================================================
//
//  实验目的：验证 "低频上报 + 云端AI预测" 方案的有效性
//
//  对比组：
//    组A: 高频上报 (1秒/次) - 基准组，数据最新鲜
//    组B: 低频上报 + AI预测 (10秒/次) - 我们的方案
//    组C: 低频上报无预测 (10秒/次) - 对照组，使用陈旧数据
//
//  实验场景：
//    - 10架UAV执行巡检任务
//    - 模拟600秒（10分钟）的运行
//    - 每隔一段时间触发一次调度决策
//    - UAV状态持续变化（电量消耗、位置移动）
//
//  评估指标：
//    1. 带宽消耗：上报次数 × 数据包大小
//    2. 调度决策误差：基于陈旧/预测数据的决策 vs 真实最优决策
//    3. 电量预测误差：预测电量 vs 真实电量
//    4. 位置预测误差：预测位置 vs 真实位置
// =====================================================================

const (
	NumUAVs           = 10              // UAV数量
	SimulationSeconds = 600             // 模拟时长（秒）
	ScheduleInterval  = 15              // 调度决策间隔（秒）
	HighFreqInterval  = 1               // 高频上报间隔（秒）
	LowFreqInterval   = 10              // 低频上报间隔（秒）
	DataPacketSize    = 512             // 每次上报的数据包大小（字节）
)

// TimeSeriesPoint 时序数据点
type TimeSeriesPoint struct {
	Time           int
	GroupA_Battery float64
	GroupB_Battery float64
	GroupC_Battery float64
	True_Battery   float64
	GroupA_PosErr  float64
	GroupB_PosErr  float64
	GroupC_PosErr  float64
}

// UAVSimulator 模拟真实UAV的状态变化
type UAVSimulator struct {
	Name     string
	Battery  float64  // 当前真实电量
	X, Y, Z  float64  // 当前真实位置
	Vx, Vy   float64  // 当前速度
	Speed    float64  // 飞行速度
	Latency  float64  // 网络延迟
	IsFlying bool     // 是否在飞行
}

// NewUAVSimulator 创建UAV模拟器
func NewUAVSimulator(name string) *UAVSimulator {
	return &UAVSimulator{
		Name:     name,
		Battery:  90 + rand.Float64()*10, // 90-100%
		X:        rand.Float64() * 1000,
		Y:        rand.Float64() * 1000,
		Z:        50 + rand.Float64()*50,
		Vx:       (rand.Float64() - 0.5) * 10,
		Vy:       (rand.Float64() - 0.5) * 10,
		Speed:    5 + rand.Float64()*10,
		Latency:  20 + rand.Float64()*30,
		IsFlying: true,
	}
}

// Step 模拟1秒的状态变化
func (u *UAVSimulator) Step() {
	if !u.IsFlying {
		return
	}

	// 电量消耗：基础消耗 + 速度相关消耗
	consumption := 0.02 + u.Speed*0.005
	u.Battery -= consumption
	if u.Battery < 0 {
		u.Battery = 0
		u.IsFlying = false
	}

	// 位置变化
	u.X += u.Vx
	u.Y += u.Vy

	// 边界反弹
	if u.X < 0 || u.X > 2000 {
		u.Vx = -u.Vx
	}
	if u.Y < 0 || u.Y > 2000 {
		u.Vy = -u.Vy
	}

	// 随机速度扰动（模拟风力等因素）
	u.Vx += (rand.Float64() - 0.5) * 1.0
	u.Vy += (rand.Float64() - 0.5) * 1.0
	u.Speed = math.Sqrt(u.Vx*u.Vx + u.Vy*u.Vy)

	// 网络延迟波动
	u.Latency = 20 + rand.Float64()*30 + rand.Float64()*20*math.Sin(float64(time.Now().UnixNano())/1e9)
	if u.Latency < 10 {
		u.Latency = 10
	}
}

// GetTrueMetrics 获取当前真实状态
func (u *UAVSimulator) GetTrueMetrics(timestamp time.Time) *models.UAVMetrics {
	return &models.UAVMetrics{
		NodeName: u.Name,
		GPS: models.GPSData{
			Speed:      u.Speed,
			LastUpdate: timestamp,
		},
		Battery: models.BatteryData{
			RemainingPercent: u.Battery,
		},
		Position: &models.PositionData{X: u.X, Y: u.Y, Z: u.Z},
		Velocity: &models.VelocityData{Vx: u.Vx, Vy: u.Vy, Vz: 0},
		Network:  &models.NetworkData{Latency: u.Latency},
	}
}

// HistoryPoint 历史数据点（用于本地预测）
type HistoryPoint struct {
	Time    time.Time
	Battery float64
	X, Y    float64
	Vx, Vy  float64
}

// ExperimentGroup 实验组
type ExperimentGroup struct {
	Name            string
	ReportInterval  int                               // 上报间隔（秒）
	UsePrediction   bool                              // 是否使用预测
	LastReportTime  map[string]time.Time              // 每个UAV上次上报时间
	LastReportData  map[string]*models.UAVMetrics     // 每个UAV上次上报的数据
	History         map[string][]HistoryPoint         // 历史数据（用于预测）
	ReportCount     int                               // 总上报次数
	BandwidthUsed   int                               // 总带宽消耗（字节）
	DecisionErrors  []float64                         // 调度决策误差
	BatteryErrors   []float64                         // 电量预测误差
	PositionErrors  []float64                         // 位置预测误差
}

// NewExperimentGroup 创建实验组
func NewExperimentGroup(name string, reportInterval int, usePrediction bool) *ExperimentGroup {
	return &ExperimentGroup{
		Name:           name,
		ReportInterval: reportInterval,
		UsePrediction:  usePrediction,
		LastReportTime: make(map[string]time.Time),
		LastReportData: make(map[string]*models.UAVMetrics),
		History:        make(map[string][]HistoryPoint),
	}
}

// ShouldReport 判断是否应该上报
func (g *ExperimentGroup) ShouldReport(uavName string, currentTime time.Time) bool {
	lastTime, exists := g.LastReportTime[uavName]
	if !exists {
		return true
	}
	return currentTime.Sub(lastTime).Seconds() >= float64(g.ReportInterval)
}

// Report 上报数据
func (g *ExperimentGroup) Report(metrics *models.UAVMetrics, currentTime time.Time) {
	g.LastReportTime[metrics.NodeName] = currentTime
	g.LastReportData[metrics.NodeName] = metrics
	g.ReportCount++
	g.BandwidthUsed += DataPacketSize

	// 记录历史数据用于预测
	point := HistoryPoint{
		Time:    currentTime,
		Battery: metrics.Battery.RemainingPercent,
		X:       metrics.Position.X,
		Y:       metrics.Position.Y,
		Vx:      metrics.Velocity.Vx,
		Vy:      metrics.Velocity.Vy,
	}
	g.History[metrics.NodeName] = append(g.History[metrics.NodeName], point)
	// 只保留最近10个点
	if len(g.History[metrics.NodeName]) > 10 {
		g.History[metrics.NodeName] = g.History[metrics.NodeName][1:]
	}
}

// GetDecisionData 获取用于调度决策的数据
func (g *ExperimentGroup) GetDecisionData(uavName string, currentTime time.Time) (battery float64, x, y float64, dataAge float64) {
	lastData, exists := g.LastReportData[uavName]
	if !exists {
		return 0, 0, 0, 999
	}

	dataAge = currentTime.Sub(lastData.GPS.LastUpdate).Seconds()

	if g.UsePrediction {
		// 使用本地预测：基于历史数据进行线性外推
		history := g.History[uavName]
		if len(history) >= 2 {
			// 计算电量消耗速率
			latest := history[len(history)-1]
			prev := history[len(history)-2]
			dt := latest.Time.Sub(prev.Time).Seconds()
			if dt > 0 {
				batteryRate := (latest.Battery - prev.Battery) / dt // 每秒消耗
				// 外推到当前时间
				battery = latest.Battery + batteryRate*dataAge
				if battery < 0 {
					battery = 0
				}
				if battery > 100 {
					battery = 100
				}

				// 位置预测：使用速度外推
				x = latest.X + latest.Vx*dataAge
				y = latest.Y + latest.Vy*dataAge
			} else {
				battery = lastData.Battery.RemainingPercent
				x = lastData.Position.X
				y = lastData.Position.Y
			}
		} else {
			// 历史数据不足，使用原值
			battery = lastData.Battery.RemainingPercent
			x = lastData.Position.X
			y = lastData.Position.Y
		}
	} else {
		// 使用原始陈旧数据
		battery = lastData.Battery.RemainingPercent
		x = lastData.Position.X
		y = lastData.Position.Y
	}
	return
}

// SchedulingDecision 调度决策：选择电量最高的UAV
func makeSchedulingDecision(batteries map[string]float64) string {
	var bestUAV string
	var maxBattery float64 = -1

	for uav, battery := range batteries {
		if battery > maxBattery {
			maxBattery = battery
			bestUAV = uav
		}
	}
	return bestUAV
}

func main() {
	fmt.Println("=" + repeatStr("=", 70))
	fmt.Println("  云端预测带宽节省效果对比实验")
	fmt.Println("=" + repeatStr("=", 70))
	fmt.Println()
	fmt.Println("实验设置：")
	fmt.Printf("  - UAV数量: %d\n", NumUAVs)
	fmt.Printf("  - 模拟时长: %d秒\n", SimulationSeconds)
	fmt.Printf("  - 调度决策间隔: %d秒\n", ScheduleInterval)
	fmt.Printf("  - 高频上报间隔: %d秒\n", HighFreqInterval)
	fmt.Printf("  - 低频上报间隔: %d秒\n", LowFreqInterval)
	fmt.Println()

	rand.Seed(time.Now().UnixNano())

	// 创建UAV模拟器
	uavs := make([]*UAVSimulator, NumUAVs)
	for i := 0; i < NumUAVs; i++ {
		uavs[i] = NewUAVSimulator(fmt.Sprintf("uav-%d", i))
	}

	// 创建三个实验组
	groupA := NewExperimentGroup("A: 高频上报(1秒)", HighFreqInterval, false)
	groupB := NewExperimentGroup("B: 低频+预测(10秒)", LowFreqInterval, true)
	groupC := NewExperimentGroup("C: 低频无预测(10秒)", LowFreqInterval, false)

	groups := []*ExperimentGroup{groupA, groupB, groupC}

	// 用于记录时序数据
	var timeSeries []TimeSeriesPoint

	// 开始模拟
	fmt.Println("开始模拟...")
	startTime := time.Now()

	for sec := 0; sec < SimulationSeconds; sec++ {
		currentTime := startTime.Add(time.Duration(sec) * time.Second)

		// 1. 更新所有UAV的真实状态
		for _, uav := range uavs {
			uav.Step()
		}

		// 2. 各组根据上报间隔决定是否上报
		for _, group := range groups {
			for _, uav := range uavs {
				if group.ShouldReport(uav.Name, currentTime) {
					metrics := uav.GetTrueMetrics(currentTime)
					group.Report(metrics, currentTime)
				}
			}
		}

		// 3. 每隔ScheduleInterval秒进行一次调度决策
		if sec > 0 && sec%ScheduleInterval == 0 {
			// 获取真实状态（作为基准）
			trueBatteries := make(map[string]float64)
			truePositions := make(map[string][2]float64)
			for _, uav := range uavs {
				trueBatteries[uav.Name] = uav.Battery
				truePositions[uav.Name] = [2]float64{uav.X, uav.Y}
			}
			trueDecision := makeSchedulingDecision(trueBatteries)

			// 各组基于自己的数据做决策
			for _, group := range groups {
				groupBatteries := make(map[string]float64)
				groupPositions := make(map[string][2]float64)

				for _, uav := range uavs {
					battery, x, y, _ := group.GetDecisionData(uav.Name, currentTime)
					groupBatteries[uav.Name] = battery
					groupPositions[uav.Name] = [2]float64{x, y}

					// 记录预测误差
					batteryErr := math.Abs(battery - trueBatteries[uav.Name])
					group.BatteryErrors = append(group.BatteryErrors, batteryErr)

					posErr := math.Sqrt(
						math.Pow(x-truePositions[uav.Name][0], 2) +
							math.Pow(y-truePositions[uav.Name][1], 2))
					group.PositionErrors = append(group.PositionErrors, posErr)
				}

				groupDecision := makeSchedulingDecision(groupBatteries)

				// 计算决策误差：选错UAV时的电量差距
				if groupDecision != trueDecision {
					decisionErr := trueBatteries[trueDecision] - trueBatteries[groupDecision]
					group.DecisionErrors = append(group.DecisionErrors, decisionErr)
				} else {
					group.DecisionErrors = append(group.DecisionErrors, 0)
				}
			}

			// 记录时序数据（取第一个UAV作为示例）
			uav0 := uavs[0]
			batA, _, _, _ := groupA.GetDecisionData(uav0.Name, currentTime)
			batB, _, _, _ := groupB.GetDecisionData(uav0.Name, currentTime)
			batC, _, _, _ := groupC.GetDecisionData(uav0.Name, currentTime)

			_, xA, yA, _ := groupA.GetDecisionData(uav0.Name, currentTime)
			_, xB, yB, _ := groupB.GetDecisionData(uav0.Name, currentTime)
			_, xC, yC, _ := groupC.GetDecisionData(uav0.Name, currentTime)

			posErrA := math.Sqrt(math.Pow(xA-uav0.X, 2) + math.Pow(yA-uav0.Y, 2))
			posErrB := math.Sqrt(math.Pow(xB-uav0.X, 2) + math.Pow(yB-uav0.Y, 2))
			posErrC := math.Sqrt(math.Pow(xC-uav0.X, 2) + math.Pow(yC-uav0.Y, 2))

			timeSeries = append(timeSeries, TimeSeriesPoint{
				Time:           sec,
				GroupA_Battery: batA,
				GroupB_Battery: batB,
				GroupC_Battery: batC,
				True_Battery:   uav0.Battery,
				GroupA_PosErr:  posErrA,
				GroupB_PosErr:  posErrB,
				GroupC_PosErr:  posErrC,
			})
		}

		// 进度显示
		if sec%100 == 0 {
			fmt.Printf("  进度: %d/%d秒 (%.1f%%)\n", sec, SimulationSeconds, float64(sec)/float64(SimulationSeconds)*100)
		}
	}

	fmt.Println("\n模拟完成，生成报告...")
	fmt.Println()

	// =====================================================================
	// 输出结果
	// =====================================================================

	fmt.Println("=" + repeatStr("=", 70))
	fmt.Println("  实验结果")
	fmt.Println("=" + repeatStr("=", 70))

	// 表1：带宽消耗对比
	fmt.Println("\n【表1】带宽消耗对比")
	fmt.Println(repeatStr("-", 70))
	fmt.Printf("%-25s %-15s %-15s %-15s\n", "实验组", "上报次数", "带宽消耗(KB)", "节省比例")
	fmt.Println(repeatStr("-", 70))

	baselineBandwidth := groupA.BandwidthUsed
	for _, g := range groups {
		saving := (1 - float64(g.BandwidthUsed)/float64(baselineBandwidth)) * 100
		savingStr := "-"
		if g != groupA {
			savingStr = fmt.Sprintf("%.1f%%", saving)
		}
		fmt.Printf("%-25s %-15d %-15.1f %-15s\n",
			g.Name, g.ReportCount, float64(g.BandwidthUsed)/1024, savingStr)
	}

	// 表2：预测误差对比
	fmt.Println("\n【表2】数据准确性对比（调度决策时刻）")
	fmt.Println(repeatStr("-", 70))
	fmt.Printf("%-25s %-18s %-18s %-15s\n", "实验组", "电量MAE(%)", "位置MAE(m)", "决策误差(%)")
	fmt.Println(repeatStr("-", 70))

	for _, g := range groups {
		batteryMAE := mean(g.BatteryErrors)
		positionMAE := mean(g.PositionErrors)
		decisionMAE := mean(g.DecisionErrors)
		fmt.Printf("%-25s %-18.3f %-18.2f %-15.3f\n",
			g.Name, batteryMAE, positionMAE, decisionMAE)
	}

	// 表3：调度决策正确率
	fmt.Println("\n【表3】调度决策正确率")
	fmt.Println(repeatStr("-", 70))
	fmt.Printf("%-25s %-15s %-15s %-20s\n", "实验组", "总决策次数", "正确次数", "正确率")
	fmt.Println(repeatStr("-", 70))

	for _, g := range groups {
		correctCount := 0
		for _, err := range g.DecisionErrors {
			if err == 0 {
				correctCount++
			}
		}
		rate := float64(correctCount) / float64(len(g.DecisionErrors)) * 100
		fmt.Printf("%-25s %-15d %-15d %-20.1f%%\n",
			g.Name, len(g.DecisionErrors), correctCount, rate)
	}

	// 核心结论
	fmt.Println("\n" + repeatStr("=", 70))
	fmt.Println("  核心结论")
	fmt.Println(repeatStr("=", 70))

	bandwidthSavingB := (1 - float64(groupB.BandwidthUsed)/float64(groupA.BandwidthUsed)) * 100
	batteryMAE_A := mean(groupA.BatteryErrors)
	batteryMAE_B := mean(groupB.BatteryErrors)
	batteryMAE_C := mean(groupC.BatteryErrors)

	fmt.Printf("\n1. 带宽节省: 方案B(低频+预测)相比方案A(高频)节省 %.1f%% 带宽\n", bandwidthSavingB)
	fmt.Printf("\n2. 预测效果:\n")
	fmt.Printf("   - 方案A(高频上报)电量误差: %.3f%%\n", batteryMAE_A)
	fmt.Printf("   - 方案B(低频+预测)电量误差: %.3f%%\n", batteryMAE_B)
	fmt.Printf("   - 方案C(低频无预测)电量误差: %.3f%%\n", batteryMAE_C)
	fmt.Printf("   - 预测使误差降低: %.1f%%\n", (batteryMAE_C-batteryMAE_B)/batteryMAE_C*100)

	correctRateA := countZeros(groupA.DecisionErrors) * 100 / len(groupA.DecisionErrors)
	correctRateB := countZeros(groupB.DecisionErrors) * 100 / len(groupB.DecisionErrors)
	correctRateC := countZeros(groupC.DecisionErrors) * 100 / len(groupC.DecisionErrors)

	fmt.Printf("\n3. 调度决策正确率:\n")
	fmt.Printf("   - 方案A(高频上报): %d%%\n", correctRateA)
	fmt.Printf("   - 方案B(低频+预测): %d%%\n", correctRateB)
	fmt.Printf("   - 方案C(低频无预测): %d%%\n", correctRateC)

	// 保存CSV文件
	saveResultsToCSV(groups, timeSeries)

	fmt.Println("\n" + repeatStr("=", 70))
	fmt.Println("  CSV文件已保存到 experiments/bandwidth_saving/")
	fmt.Println(repeatStr("=", 70))
}

func saveResultsToCSV(groups []*ExperimentGroup, timeSeries []TimeSeriesPoint) {
	// 保存汇总结果
	summaryFile, _ := os.Create("experiments/bandwidth_saving/summary.csv")
	defer summaryFile.Close()
	summaryWriter := csv.NewWriter(summaryFile)
	defer summaryWriter.Flush()

	summaryWriter.Write([]string{"group", "report_count", "bandwidth_kb", "battery_mae", "position_mae", "decision_mae", "decision_correct_rate"})

	for _, g := range groups {
		correctCount := countZeros(g.DecisionErrors)
		correctRate := float64(correctCount) / float64(len(g.DecisionErrors)) * 100

		summaryWriter.Write([]string{
			g.Name,
			fmt.Sprintf("%d", g.ReportCount),
			fmt.Sprintf("%.2f", float64(g.BandwidthUsed)/1024),
			fmt.Sprintf("%.4f", mean(g.BatteryErrors)),
			fmt.Sprintf("%.4f", mean(g.PositionErrors)),
			fmt.Sprintf("%.4f", mean(g.DecisionErrors)),
			fmt.Sprintf("%.2f", correctRate),
		})
	}

	// 保存时序数据
	timeSeriesFile, _ := os.Create("experiments/bandwidth_saving/timeseries.csv")
	defer timeSeriesFile.Close()
	tsWriter := csv.NewWriter(timeSeriesFile)
	defer tsWriter.Flush()

	tsWriter.Write([]string{"time_sec", "true_battery", "groupA_battery", "groupB_battery", "groupC_battery",
		"groupA_pos_err", "groupB_pos_err", "groupC_pos_err"})

	for _, p := range timeSeries {
		tsWriter.Write([]string{
			fmt.Sprintf("%d", p.Time),
			fmt.Sprintf("%.4f", p.True_Battery),
			fmt.Sprintf("%.4f", p.GroupA_Battery),
			fmt.Sprintf("%.4f", p.GroupB_Battery),
			fmt.Sprintf("%.4f", p.GroupC_Battery),
			fmt.Sprintf("%.4f", p.GroupA_PosErr),
			fmt.Sprintf("%.4f", p.GroupB_PosErr),
			fmt.Sprintf("%.4f", p.GroupC_PosErr),
		})
	}

	// 保存详细误差数据
	for _, g := range groups {
		// 保存每UAV误差数据
		filename := fmt.Sprintf("experiments/bandwidth_saving/errors_%s.csv", sanitizeFilename(g.Name))
		file, _ := os.Create(filename)
		writer := csv.NewWriter(file)

		writer.Write([]string{"index", "battery_error", "position_error"})
		for i := range g.BatteryErrors {
			writer.Write([]string{
				fmt.Sprintf("%d", i),
				fmt.Sprintf("%.4f", g.BatteryErrors[i]),
				fmt.Sprintf("%.4f", g.PositionErrors[i]),
			})
		}
		writer.Flush()
		file.Close()

		// 保存决策误差数据
		decFilename := fmt.Sprintf("experiments/bandwidth_saving/decisions_%s.csv", sanitizeFilename(g.Name))
		decFile, _ := os.Create(decFilename)
		decWriter := csv.NewWriter(decFile)

		decWriter.Write([]string{"decision_index", "decision_error"})
		for i, err := range g.DecisionErrors {
			decWriter.Write([]string{
				fmt.Sprintf("%d", i),
				fmt.Sprintf("%.4f", err),
			})
		}
		decWriter.Flush()
		decFile.Close()
	}
}

func sanitizeFilename(s string) string {
	result := ""
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			result += string(c)
		}
	}
	return result
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

func countZeros(values []float64) int {
	count := 0
	for _, v := range values {
		if v == 0 {
			count++
		}
	}
	return count
}

func repeatStr(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

// 以下是用于分析的辅助函数

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	index := int(float64(len(sorted)-1) * p / 100)
	return sorted[index]
}

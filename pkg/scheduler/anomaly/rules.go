package anomaly

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// RuleBasedDetector 基于规则的异常检测器
// 使用预定义的阈值和规则检测已知类型的异常
type RuleBasedDetector struct {
	config *DetectorConfig

	// 缓存每个节点的上一次数据
	lastMetrics map[string]*metricsCache
	mu          sync.RWMutex
}

// metricsCache 缓存的指标数据
type metricsCache struct {
	Metrics   *models.UAVMetrics
	Timestamp time.Time
}

// NewRuleBasedDetector 创建规则检测器
func NewRuleBasedDetector(config *DetectorConfig) *RuleBasedDetector {
	return &RuleBasedDetector{
		config:      config,
		lastMetrics: make(map[string]*metricsCache),
	}
}

// Name 返回检测器名称
func (rd *RuleBasedDetector) Name() string {
	return "rule_based"
}

// Detect 执行规则检测
func (rd *RuleBasedDetector) Detect(metrics *models.UAVMetrics) []*Anomaly {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	var anomalies []*Anomaly

	nodeName := metrics.NodeName
	now := time.Now()

	// 获取上一次的数据
	lastCache := rd.lastMetrics[nodeName]

	// 1. 电池相关检测
	batteryAnomalies := rd.checkBattery(metrics, lastCache)
	anomalies = append(anomalies, batteryAnomalies...)

	// 2. 位置相关检测
	positionAnomalies := rd.checkPosition(metrics, lastCache)
	anomalies = append(anomalies, positionAnomalies...)

	// 3. 网络相关检测
	networkAnomalies := rd.checkNetwork(metrics, lastCache)
	anomalies = append(anomalies, networkAnomalies...)

	// 4. 性能相关检测
	performanceAnomalies := rd.checkPerformance(metrics)
	anomalies = append(anomalies, performanceAnomalies...)

	// 更新缓存
	rd.lastMetrics[nodeName] = &metricsCache{
		Metrics:   metrics,
		Timestamp: now,
	}

	return anomalies
}

// checkBattery 检查电池异常
func (rd *RuleBasedDetector) checkBattery(metrics *models.UAVMetrics, lastCache *metricsCache) []*Anomaly {
	var anomalies []*Anomaly
	battery := metrics.Battery.RemainingPercent
	nodeName := metrics.NodeName

	// 检查电量过低
	if battery <= rd.config.BatteryCriticalThreshold {
		anomalies = append(anomalies, &Anomaly{
			ID:           fmt.Sprintf("battery-critical-%s-%d", nodeName, time.Now().UnixNano()),
			NodeName:     nodeName,
			Type:         AnomalyBatteryCritical,
			Severity:     SeverityFatal,
			Score:        1.0,
			Message:      fmt.Sprintf("电量危急: %.1f%% (阈值: %.1f%%)", battery, rd.config.BatteryCriticalThreshold),
			DetectedAt:   time.Now(),
			DetectedBy:   rd.Name(),
			CurrentValue: battery,
			Threshold:    rd.config.BatteryCriticalThreshold,
		})
	} else if battery <= rd.config.BatteryLowThreshold {
		anomalies = append(anomalies, &Anomaly{
			ID:           fmt.Sprintf("battery-low-%s-%d", nodeName, time.Now().UnixNano()),
			NodeName:     nodeName,
			Type:         AnomalyBatteryLow,
			Severity:     SeverityWarning,
			Score:        0.6,
			Message:      fmt.Sprintf("电量过低: %.1f%% (阈值: %.1f%%)", battery, rd.config.BatteryLowThreshold),
			DetectedAt:   time.Now(),
			DetectedBy:   rd.Name(),
			CurrentValue: battery,
			Threshold:    rd.config.BatteryLowThreshold,
		})
	}

	// 检查电量骤降
	if lastCache != nil {
		lastBattery := lastCache.Metrics.Battery.RemainingPercent
		timeDiff := time.Since(lastCache.Timestamp).Seconds()

		if timeDiff > 0 {
			dropRate := (lastBattery - battery) / timeDiff // %/s

			if dropRate >= rd.config.BatteryDropThreshold {
				anomalies = append(anomalies, &Anomaly{
					ID:            fmt.Sprintf("battery-drop-%s-%d", nodeName, time.Now().UnixNano()),
					NodeName:      nodeName,
					Type:          AnomalyBatteryDrop,
					Severity:      SeverityCritical,
					Score:         math.Min(dropRate/rd.config.BatteryDropThreshold, 1.0),
					Message:       fmt.Sprintf("电量骤降: %.2f%%/s (从%.1f%%降到%.1f%%)", dropRate, lastBattery, battery),
					DetectedAt:    time.Now(),
					DetectedBy:    rd.Name(),
					CurrentValue:  dropRate,
					ExpectedValue: 0,
					Threshold:     rd.config.BatteryDropThreshold,
				})
			}

			// 检查电量异常上升（虚电）
			if battery > lastBattery+1.0 && timeDiff < 60 { // 1分钟内上升超过1%
				anomalies = append(anomalies, &Anomaly{
					ID:           fmt.Sprintf("battery-spike-%s-%d", nodeName, time.Now().UnixNano()),
					NodeName:     nodeName,
					Type:         AnomalyBatterySpike,
					Severity:     SeverityWarning,
					Score:        0.5,
					Message:      fmt.Sprintf("电量异常上升(可能虚电): 从%.1f%%升到%.1f%%", lastBattery, battery),
					DetectedAt:   time.Now(),
					DetectedBy:   rd.Name(),
					CurrentValue: battery - lastBattery,
				})
			}
		}
	}

	return anomalies
}

// checkPosition 检查位置异常
func (rd *RuleBasedDetector) checkPosition(metrics *models.UAVMetrics, lastCache *metricsCache) []*Anomaly {
	var anomalies []*Anomaly

	if metrics.Position == nil || lastCache == nil || lastCache.Metrics.Position == nil {
		return anomalies
	}

	nodeName := metrics.NodeName
	timeDiff := time.Since(lastCache.Timestamp).Seconds()

	if timeDiff <= 0 {
		return anomalies
	}

	// 计算位置变化
	dx := metrics.Position.X - lastCache.Metrics.Position.X
	dy := metrics.Position.Y - lastCache.Metrics.Position.Y
	dz := metrics.Position.Z - lastCache.Metrics.Position.Z
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// 计算速度
	speed := distance / timeDiff

	// 检查位置突变（考虑合理的最大速度，假设最大30m/s）
	maxReasonableSpeed := 30.0
	if speed > maxReasonableSpeed && distance > rd.config.PositionJumpThreshold {
		anomalies = append(anomalies, &Anomaly{
			ID:            fmt.Sprintf("position-jump-%s-%d", nodeName, time.Now().UnixNano()),
			NodeName:      nodeName,
			Type:          AnomalyPositionJump,
			Severity:      SeverityCritical,
			Score:         math.Min(speed/100, 1.0),
			Message:       fmt.Sprintf("位置突变(可能GPS漂移): 移动%.1f米 (速度%.1fm/s)", distance, speed),
			DetectedAt:    time.Now(),
			DetectedBy:    rd.Name(),
			CurrentValue:  distance,
			ExpectedValue: 0,
			Threshold:     rd.config.PositionJumpThreshold,
		})
	}

	// 检查位置卡住（长时间无移动但应该在飞行）
	if metrics.Flight != nil && metrics.Flight.IsFlying && distance < 0.5 && timeDiff > 10 {
		anomalies = append(anomalies, &Anomaly{
			ID:         fmt.Sprintf("position-stuck-%s-%d", nodeName, time.Now().UnixNano()),
			NodeName:   nodeName,
			Type:       AnomalyPositionStuck,
			Severity:   SeverityWarning,
			Score:      0.5,
			Message:    fmt.Sprintf("飞行中位置卡住: %.1f秒内仅移动%.2f米", timeDiff, distance),
			DetectedAt: time.Now(),
			DetectedBy: rd.Name(),
		})
	}

	return anomalies
}

// checkNetwork 检查网络异常
func (rd *RuleBasedDetector) checkNetwork(metrics *models.UAVMetrics, lastCache *metricsCache) []*Anomaly {
	var anomalies []*Anomaly

	if metrics.Network == nil {
		return anomalies
	}

	nodeName := metrics.NodeName
	latency := metrics.Network.Latency

	// 检查延迟突增
	if latency >= rd.config.LatencySpikeThreshold {
		anomalies = append(anomalies, &Anomaly{
			ID:           fmt.Sprintf("latency-spike-%s-%d", nodeName, time.Now().UnixNano()),
			NodeName:     nodeName,
			Type:         AnomalyLatencySpike,
			Severity:     SeverityCritical,
			Score:        math.Min(latency/1000, 1.0),
			Message:      fmt.Sprintf("网络延迟过高: %.1fms (阈值: %.1fms)", latency, rd.config.LatencySpikeThreshold),
			DetectedAt:   time.Now(),
			DetectedBy:   rd.Name(),
			CurrentValue: latency,
			Threshold:    rd.config.LatencySpikeThreshold,
		})
	}

	// 检查丢包率
	if metrics.Network.PacketLoss >= 10.0 { // 10%丢包
		severity := SeverityWarning
		if metrics.Network.PacketLoss >= 30.0 {
			severity = SeverityCritical
		}
		anomalies = append(anomalies, &Anomaly{
			ID:           fmt.Sprintf("packet-loss-%s-%d", nodeName, time.Now().UnixNano()),
			NodeName:     nodeName,
			Type:         AnomalyPacketLoss,
			Severity:     severity,
			Score:        math.Min(metrics.Network.PacketLoss/100, 1.0),
			Message:      fmt.Sprintf("丢包率过高: %.1f%%", metrics.Network.PacketLoss),
			DetectedAt:   time.Now(),
			DetectedBy:   rd.Name(),
			CurrentValue: metrics.Network.PacketLoss,
		})
	}

	return anomalies
}

// checkPerformance 检查性能异常
func (rd *RuleBasedDetector) checkPerformance(metrics *models.UAVMetrics) []*Anomaly {
	var anomalies []*Anomaly

	if metrics.Performance == nil {
		return anomalies
	}

	nodeName := metrics.NodeName
	perf := metrics.Performance

	// 检查CPU
	if perf.CPUUsage >= rd.config.CPUHighThreshold {
		anomalies = append(anomalies, &Anomaly{
			ID:           fmt.Sprintf("cpu-high-%s-%d", nodeName, time.Now().UnixNano()),
			NodeName:     nodeName,
			Type:         AnomalyCPUHigh,
			Severity:     SeverityWarning,
			Score:        perf.CPUUsage / 100,
			Message:      fmt.Sprintf("CPU使用率过高: %.1f%% (阈值: %.1f%%)", perf.CPUUsage, rd.config.CPUHighThreshold),
			DetectedAt:   time.Now(),
			DetectedBy:   rd.Name(),
			CurrentValue: perf.CPUUsage,
			Threshold:    rd.config.CPUHighThreshold,
		})
	}

	// 检查内存
	if perf.MemoryUsage >= rd.config.MemoryHighThreshold {
		anomalies = append(anomalies, &Anomaly{
			ID:           fmt.Sprintf("memory-high-%s-%d", nodeName, time.Now().UnixNano()),
			NodeName:     nodeName,
			Type:         AnomalyMemoryHigh,
			Severity:     SeverityWarning,
			Score:        perf.MemoryUsage / 100,
			Message:      fmt.Sprintf("内存使用率过高: %.1f%% (阈值: %.1f%%)", perf.MemoryUsage, rd.config.MemoryHighThreshold),
			DetectedAt:   time.Now(),
			DetectedBy:   rd.Name(),
			CurrentValue: perf.MemoryUsage,
			Threshold:    rd.config.MemoryHighThreshold,
		})
	}

	// 检查温度
	if perf.Temperature >= rd.config.TemperatureHighThreshold {
		severity := SeverityWarning
		if perf.Temperature >= 80 {
			severity = SeverityCritical
		}
		anomalies = append(anomalies, &Anomaly{
			ID:           fmt.Sprintf("temp-high-%s-%d", nodeName, time.Now().UnixNano()),
			NodeName:     nodeName,
			Type:         AnomalyTemperatureHigh,
			Severity:     severity,
			Score:        math.Min(perf.Temperature/100, 1.0),
			Message:      fmt.Sprintf("温度过高: %.1f°C (阈值: %.1f°C)", perf.Temperature, rd.config.TemperatureHighThreshold),
			DetectedAt:   time.Now(),
			DetectedBy:   rd.Name(),
			CurrentValue: perf.Temperature,
			Threshold:    rd.config.TemperatureHighThreshold,
		})
	}

	return anomalies
}

// Reset 重置检测器
func (rd *RuleBasedDetector) Reset() {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	rd.lastMetrics = make(map[string]*metricsCache)
}

// ResetNode 重置单个节点
func (rd *RuleBasedDetector) ResetNode(nodeName string) {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	delete(rd.lastMetrics, nodeName)
}

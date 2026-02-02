package anomaly

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// StatisticalDetector 统计方法异常检测器
// 使用滑动窗口计算均值和标准差，通过Z-score检测异常
type StatisticalDetector struct {
	config *DetectorConfig

	// 每个节点每个指标的滑动窗口
	windows map[string]map[string]*SlidingWindow
	mu      sync.RWMutex
}

// SlidingWindow 滑动窗口
type SlidingWindow struct {
	Values   []float64
	MaxSize  int
	head     int
	count    int
	sum      float64
	sumSq    float64 // 平方和，用于计算方差
}

// NewStatisticalDetector 创建统计检测器
func NewStatisticalDetector(config *DetectorConfig) *StatisticalDetector {
	return &StatisticalDetector{
		config:  config,
		windows: make(map[string]map[string]*SlidingWindow),
	}
}

// Name 返回检测器名称
func (sd *StatisticalDetector) Name() string {
	return "statistical"
}

// NewSlidingWindow 创建滑动窗口
func NewSlidingWindow(maxSize int) *SlidingWindow {
	return &SlidingWindow{
		Values:  make([]float64, maxSize),
		MaxSize: maxSize,
	}
}

// Add 添加新值
func (sw *SlidingWindow) Add(value float64) {
	if sw.count >= sw.MaxSize {
		// 移除最旧的值
		oldValue := sw.Values[sw.head]
		sw.sum -= oldValue
		sw.sumSq -= oldValue * oldValue
	} else {
		sw.count++
	}

	sw.Values[sw.head] = value
	sw.sum += value
	sw.sumSq += value * value
	sw.head = (sw.head + 1) % sw.MaxSize
}

// Mean 计算均值
func (sw *SlidingWindow) Mean() float64 {
	if sw.count == 0 {
		return 0
	}
	return sw.sum / float64(sw.count)
}

// StdDev 计算标准差
func (sw *SlidingWindow) StdDev() float64 {
	if sw.count < 2 {
		return 0
	}
	mean := sw.Mean()
	variance := sw.sumSq/float64(sw.count) - mean*mean
	if variance < 0 {
		variance = 0 // 数值误差修正
	}
	return math.Sqrt(variance)
}

// ZScore 计算Z-score
func (sw *SlidingWindow) ZScore(value float64) float64 {
	stdDev := sw.StdDev()
	if stdDev == 0 {
		return 0
	}
	return (value - sw.Mean()) / stdDev
}

// Count 返回当前数据点数
func (sw *SlidingWindow) Count() int {
	return sw.count
}

// getOrCreateWindow 获取或创建指标的滑动窗口
func (sd *StatisticalDetector) getOrCreateWindow(nodeName, metric string) *SlidingWindow {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	if _, exists := sd.windows[nodeName]; !exists {
		sd.windows[nodeName] = make(map[string]*SlidingWindow)
	}

	if _, exists := sd.windows[nodeName][metric]; !exists {
		sd.windows[nodeName][metric] = NewSlidingWindow(sd.config.StatisticalWindowSize)
	}

	return sd.windows[nodeName][metric]
}

// CheckMetric 检查单个指标是否异常
func (sd *StatisticalDetector) CheckMetric(nodeName, metric string, value float64) *Anomaly {
	window := sd.getOrCreateWindow(nodeName, metric)

	// 需要足够的历史数据
	if window.Count() < 5 {
		window.Add(value)
		return nil
	}

	// 计算Z-score
	zScore := window.ZScore(value)
	absZScore := math.Abs(zScore)

	// 添加到窗口
	window.Add(value)

	// 检查是否超过阈值
	if absZScore >= sd.config.ZScoreThreshold {
		severity := sd.determineSeverity(absZScore)
		anomalyType := sd.determineAnomalyType(metric, zScore)

		return &Anomaly{
			ID:            fmt.Sprintf("%s-%s-%d", nodeName, metric, time.Now().UnixNano()),
			NodeName:      nodeName,
			Type:          anomalyType,
			Severity:      severity,
			Score:         math.Min(absZScore/10.0, 1.0), // 归一化到0-1
			Message:       fmt.Sprintf("%s异常: Z-score=%.2f, 当前值=%.2f, 均值=%.2f, 标准差=%.2f", metric, zScore, value, window.Mean(), window.StdDev()),
			DetectedAt:    time.Now(),
			DetectedBy:    sd.Name(),
			CurrentValue:  value,
			ExpectedValue: window.Mean(),
			Threshold:     sd.config.ZScoreThreshold,
		}
	}

	return nil
}

// determineSeverity 根据Z-score确定严重程度
func (sd *StatisticalDetector) determineSeverity(absZScore float64) AnomalySeverity {
	switch {
	case absZScore >= 5.0:
		return SeverityFatal
	case absZScore >= 4.0:
		return SeverityCritical
	case absZScore >= 3.0:
		return SeverityWarning
	default:
		return SeverityInfo
	}
}

// determineAnomalyType 根据指标名称确定异常类型
func (sd *StatisticalDetector) determineAnomalyType(metric string, zScore float64) AnomalyType {
	switch metric {
	case "battery":
		if zScore < 0 {
			return AnomalyBatteryDrop
		}
		return AnomalyBatterySpike
	case "position_change":
		return AnomalyPositionJump
	case "latency":
		return AnomalyLatencySpike
	case "cpu":
		return AnomalyCPUHigh
	case "memory":
		return AnomalyMemoryHigh
	case "temperature":
		return AnomalyTemperatureHigh
	default:
		return AnomalyUnknown
	}
}

// GetWindowStats 获取窗口统计信息
func (sd *StatisticalDetector) GetWindowStats(nodeName, metric string) map[string]interface{} {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	if nodeWindows, exists := sd.windows[nodeName]; exists {
		if window, exists := nodeWindows[metric]; exists {
			return map[string]interface{}{
				"count":   window.Count(),
				"mean":    window.Mean(),
				"std_dev": window.StdDev(),
			}
		}
	}

	return nil
}

// Reset 重置检测器
func (sd *StatisticalDetector) Reset() {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.windows = make(map[string]map[string]*SlidingWindow)
}

// ResetNode 重置单个节点
func (sd *StatisticalDetector) ResetNode(nodeName string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	delete(sd.windows, nodeName)
}

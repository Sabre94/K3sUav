package predictor

import (
	"log"
	"math"
	"sync"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// StatePredictor 状态预测器
// 整合电池、位置、延迟预测，为调度器提供"预测增强"的UAVMetrics
type StatePredictor struct {
	config *PredictorConfig

	// 历史数据管理
	history *HistoryManager

	// 子预测器
	batteryPredictor  *BatteryPredictor
	positionPredictor *PositionPredictor
	latencyPredictor  *LatencyPredictor

	// 统计
	stats *PredictionStats
	mu    sync.RWMutex

	// 日志
	verbose bool
}

// NewStatePredictor 创建状态预测器
func NewStatePredictor(config *PredictorConfig) *StatePredictor {
	if config == nil {
		config = DefaultConfig()
	}

	return &StatePredictor{
		config:            config,
		history:           NewHistoryManager(config),
		batteryPredictor:  NewBatteryPredictor(config),
		positionPredictor: NewPositionPredictor(config),
		latencyPredictor:  NewLatencyPredictor(config),
		stats:             &PredictionStats{},
		verbose:           false,
	}
}

// SetVerbose 设置是否输出详细日志
func (sp *StatePredictor) SetVerbose(verbose bool) {
	sp.verbose = verbose
}

// UpdateHistory 更新历史数据（每次获取新数据时调用）
func (sp *StatePredictor) UpdateHistory(metrics *models.UAVMetrics) {
	sp.history.UpdateFromMetrics(metrics)
}

// UpdateHistoryBatch 批量更新历史数据
func (sp *StatePredictor) UpdateHistoryBatch(metricsList []*models.UAVMetrics) {
	sp.history.UpdateAllFromMetrics(metricsList)
}

// EnhanceMetrics 对单个UAVMetrics进行预测增强
func (sp *StatePredictor) EnhanceMetrics(metrics *models.UAVMetrics) *PredictedMetrics {
	now := time.Now()

	// 获取数据年龄
	dataAge := sp.getDataAge(metrics)

	// 创建预测结果
	predicted := &PredictedMetrics{
		UAVMetrics:     metrics,
		PredictionTime: now,
		DataAge:        dataAge,
		UsedPrediction: false,
	}

	// 如果数据足够新鲜，直接使用原值
	if dataAge < sp.config.PredictionThreshold {
		predicted.PredictedBattery = metrics.Battery.RemainingPercent
		predicted.BatteryConfidence = 1.0

		if metrics.Position != nil {
			predicted.PredictedPosition = &Position{
				X: metrics.Position.X,
				Y: metrics.Position.Y,
				Z: metrics.Position.Z,
			}
		}
		predicted.PositionConfidence = 1.0

		if metrics.Network != nil {
			predicted.PredictedLatency = metrics.Network.Latency
		}
		predicted.LatencyConfidence = 1.0

		return predicted
	}

	// 数据陈旧，启用预测
	predicted.UsedPrediction = true

	// 更新统计
	sp.mu.Lock()
	sp.stats.TotalPredictions++
	sp.mu.Unlock()

	// 获取历史缓冲区
	buffer, exists := sp.history.GetBuffer(metrics.NodeName)
	if !exists {
		// 没有历史数据，使用原值但降低置信度
		sp.setFallbackValues(predicted, metrics, dataAge)
		return predicted
	}

	// 先更新历史（确保最新数据已加入）
	buffer.AddFromMetrics(metrics, metrics.GPS.LastUpdate)

	// 电池预测
	sp.predictBattery(predicted, buffer, now)

	// 位置预测
	sp.predictPosition(predicted, buffer, now)

	// 延迟预测
	sp.predictLatency(predicted, buffer, now)

	if sp.verbose {
		log.Printf("[Predictor] Node=%s DataAge=%.1fs Battery: %.1f%% -> %.1f%% (conf=%.2f)",
			metrics.NodeName, dataAge.Seconds(),
			metrics.Battery.RemainingPercent, predicted.PredictedBattery,
			predicted.BatteryConfidence)
	}

	return predicted
}

// EnhanceMetricsBatch 批量预测增强
func (sp *StatePredictor) EnhanceMetricsBatch(metricsList []*models.UAVMetrics) []*PredictedMetrics {
	// 先更新所有历史数据
	sp.UpdateHistoryBatch(metricsList)

	// 然后进行预测
	results := make([]*PredictedMetrics, len(metricsList))
	for i, metrics := range metricsList {
		results[i] = sp.EnhanceMetrics(metrics)
	}
	return results
}

// getDataAge 获取数据年龄
func (sp *StatePredictor) getDataAge(metrics *models.UAVMetrics) time.Duration {
	lastUpdate := metrics.GPS.LastUpdate
	if lastUpdate.IsZero() {
		// 如果没有时间戳，假设数据是新鲜的
		return 0
	}
	return time.Since(lastUpdate)
}

// setFallbackValues 设置回退值（没有历史数据时）
func (sp *StatePredictor) setFallbackValues(predicted *PredictedMetrics, metrics *models.UAVMetrics, dataAge time.Duration) {
	// 使用原值
	predicted.PredictedBattery = metrics.Battery.RemainingPercent

	if metrics.Position != nil {
		predicted.PredictedPosition = &Position{
			X: metrics.Position.X,
			Y: metrics.Position.Y,
			Z: metrics.Position.Z,
		}
	}

	if metrics.Network != nil {
		predicted.PredictedLatency = metrics.Network.Latency
	}

	// 置信度随数据年龄衰减
	predicted.BatteryConfidence = sp.calculateConfidenceDecay(dataAge, sp.config.BatteryConfidenceHalfLife)
	predicted.PositionConfidence = sp.calculateConfidenceDecay(dataAge, sp.config.PositionConfidenceHalfLife)
	predicted.LatencyConfidence = sp.calculateConfidenceDecay(dataAge, sp.config.LatencyConfidenceHalfLife)
}

// predictBattery 预测电池
func (sp *StatePredictor) predictBattery(predicted *PredictedMetrics, buffer *HistoryBuffer, targetTime time.Time) {
	battery, confidence := sp.batteryPredictor.Predict(buffer, targetTime)

	predicted.PredictedBattery = battery
	predicted.BatteryConfidence = confidence

	sp.mu.Lock()
	sp.stats.BatteryPredictions++
	sp.mu.Unlock()
}

// predictPosition 预测位置
func (sp *StatePredictor) predictPosition(predicted *PredictedMetrics, buffer *HistoryBuffer, targetTime time.Time) {
	position, confidence := sp.positionPredictor.Predict(buffer, targetTime)

	predicted.PredictedPosition = position
	predicted.PositionConfidence = confidence

	sp.mu.Lock()
	sp.stats.PositionPredictions++
	sp.mu.Unlock()
}

// predictLatency 预测延迟
func (sp *StatePredictor) predictLatency(predicted *PredictedMetrics, buffer *HistoryBuffer, targetTime time.Time) {
	latency, confidence := sp.latencyPredictor.Predict(buffer, targetTime)

	predicted.PredictedLatency = latency
	predicted.LatencyConfidence = confidence

	sp.mu.Lock()
	sp.stats.LatencyPredictions++
	sp.mu.Unlock()
}

// calculateConfidenceDecay 计算置信度衰减
func (sp *StatePredictor) calculateConfidenceDecay(age time.Duration, halfLife time.Duration) float64 {
	if halfLife <= 0 {
		return 1.0
	}
	lambda := math.Log(2) / halfLife.Seconds()
	return math.Exp(-lambda * age.Seconds())
}

// FeedbackActual 反馈实际值（用于在线学习）
func (sp *StatePredictor) FeedbackActual(nodeName string, predicted *PredictedMetrics, actual *models.UAVMetrics) {
	if predicted == nil || actual == nil {
		return
	}

	// 更新电池预测器
	sp.batteryPredictor.UpdateWithActual(predicted.PredictedBattery, actual.Battery.RemainingPercent)

	// 更新位置预测器
	if predicted.PredictedPosition != nil && actual.Position != nil {
		actualPos := &Position{
			X: actual.Position.X,
			Y: actual.Position.Y,
			Z: actual.Position.Z,
		}
		sp.positionPredictor.UpdateWithActual(nodeName, predicted.PredictedPosition, actualPos)
	}

	// 更新延迟预测器
	if actual.Network != nil {
		sp.latencyPredictor.UpdateWithActual(predicted.PredictedLatency, actual.Network.Latency)
	}

	// 更新统计
	sp.mu.Lock()
	sp.stats.BatteryMAE = sp.batteryPredictor.GetMAE()
	sp.stats.PositionMAE = sp.positionPredictor.GetMAE()
	sp.stats.LatencyMAE = sp.latencyPredictor.GetMAE()
	sp.mu.Unlock()
}

// GetStats 获取预测统计
func (sp *StatePredictor) GetStats() *PredictionStats {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	// 复制一份返回
	stats := *sp.stats
	return &stats
}

// GetDetailedStats 获取详细统计信息
func (sp *StatePredictor) GetDetailedStats() map[string]interface{} {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	return map[string]interface{}{
		"total_predictions": sp.stats.TotalPredictions,
		"battery": map[string]interface{}{
			"predictions": sp.stats.BatteryPredictions,
			"mae":         sp.stats.BatteryMAE,
			"details":     sp.batteryPredictor.GetStats(),
		},
		"position": map[string]interface{}{
			"predictions": sp.stats.PositionPredictions,
			"mae_meters":  sp.stats.PositionMAE,
			"details":     sp.positionPredictor.GetStats(),
		},
		"latency": map[string]interface{}{
			"predictions": sp.stats.LatencyPredictions,
			"mae_ms":      sp.stats.LatencyMAE,
			"details":     sp.latencyPredictor.GetStats(),
		},
		"history": map[string]interface{}{
			"node_count": sp.history.GetNodeCount(),
			"nodes":      sp.history.GetAllNodeNames(),
		},
		"config": map[string]interface{}{
			"history_size":         sp.config.HistorySize,
			"max_data_age":         sp.config.MaxDataAge.String(),
			"prediction_threshold": sp.config.PredictionThreshold.String(),
			"lstm_enabled":         sp.config.LSTMEnabled,
		},
	}
}

// Reset 重置预测器状态
func (sp *StatePredictor) Reset() {
	sp.history.Clear()
	sp.latencyPredictor.Reset()

	sp.mu.Lock()
	sp.stats = &PredictionStats{}
	sp.mu.Unlock()
}

// GetConfig 获取配置
func (sp *StatePredictor) GetConfig() *PredictorConfig {
	return sp.config
}

// --- 便捷方法：直接获取预测值 ---

// GetPredictedBattery 获取预测电量
func (sp *StatePredictor) GetPredictedBattery(nodeName string) (float64, float64, bool) {
	buffer, exists := sp.history.GetBuffer(nodeName)
	if !exists {
		return 0, 0, false
	}

	battery, confidence := sp.batteryPredictor.Predict(buffer, time.Now())
	return battery, confidence, true
}

// GetPredictedPosition 获取预测位置
func (sp *StatePredictor) GetPredictedPosition(nodeName string) (*Position, float64, bool) {
	buffer, exists := sp.history.GetBuffer(nodeName)
	if !exists {
		return nil, 0, false
	}

	position, confidence := sp.positionPredictor.Predict(buffer, time.Now())
	return position, confidence, true
}

// GetPredictedLatency 获取预测延迟
func (sp *StatePredictor) GetPredictedLatency(nodeName string) (float64, float64, bool) {
	buffer, exists := sp.history.GetBuffer(nodeName)
	if !exists {
		return 0, 0, false
	}

	latency, confidence := sp.latencyPredictor.Predict(buffer, time.Now())
	return latency, confidence, true
}

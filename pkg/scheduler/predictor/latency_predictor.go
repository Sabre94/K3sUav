package predictor

import (
	"math"
	"sync"
	"time"
)

// LatencyPredictor 网络延迟预测器
// 使用指数加权移动平均 (EWMA) + 趋势分析
type LatencyPredictor struct {
	config *PredictorConfig

	// 每个节点的EWMA状态
	ewmaStates map[string]*EWMAState
	mu         sync.RWMutex

	// 统计
	predictionCount int64
	errorSum        float64
	errorCount      int64
}

// EWMAState EWMA状态
type EWMAState struct {
	// EWMA值
	Value float64

	// 趋势（变化率）
	Trend float64

	// 方差估计（用于置信度计算）
	Variance float64

	// 上次更新时间
	LastUpdate time.Time

	// 是否已初始化
	Initialized bool

	// 平滑因子
	Alpha float64 // 值平滑
	Beta  float64 // 趋势平滑
	Gamma float64 // 方差平滑
}

// NewLatencyPredictor 创建延迟预测器
func NewLatencyPredictor(config *PredictorConfig) *LatencyPredictor {
	return &LatencyPredictor{
		config:     config,
		ewmaStates: make(map[string]*EWMAState),
	}
}

// Predict 预测指定时间的网络延迟
func (lp *LatencyPredictor) Predict(history *HistoryBuffer, targetTime time.Time) (float64, float64) {
	lp.mu.Lock()
	lp.predictionCount++
	lp.mu.Unlock()

	// 获取或创建EWMA状态
	state := lp.getOrCreateState(history.NodeName)

	// 获取历史数据
	points := history.GetLastN(20)
	if len(points) == 0 {
		return 0, 0
	}

	// 更新EWMA状态
	for _, point := range points {
		if point.Latency > 0 {
			state.Update(point.Latency, point.Timestamp)
		}
	}

	if !state.Initialized {
		// 数据不足，返回最新值
		latest := points[len(points)-1]
		return latest.Latency, 0.5
	}

	// 计算预测时间差
	deltaT := targetTime.Sub(state.LastUpdate).Seconds()
	if deltaT <= 0 {
		return state.Value, 1.0
	}

	// 预测：当前值 + 趋势 * 时间
	predicted := state.Value + state.Trend*deltaT

	// 确保非负
	if predicted < 0 {
		predicted = 0
	}

	// 置信度：基于方差和预测距离
	confidence := lp.calculateConfidence(state, deltaT)

	return predicted, confidence
}

// getOrCreateState 获取或创建EWMA状态
func (lp *LatencyPredictor) getOrCreateState(nodeName string) *EWMAState {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	if state, exists := lp.ewmaStates[nodeName]; exists {
		return state
	}

	state := &EWMAState{
		Alpha: 0.3,  // 值平滑因子
		Beta:  0.1,  // 趋势平滑因子
		Gamma: 0.2,  // 方差平滑因子
	}
	lp.ewmaStates[nodeName] = state
	return state
}

// Update 更新EWMA状态
func (s *EWMAState) Update(value float64, timestamp time.Time) {
	if !s.Initialized {
		s.Value = value
		s.Trend = 0
		s.Variance = 0
		s.LastUpdate = timestamp
		s.Initialized = true
		return
	}

	// 计算时间差（用于调整平滑因子）
	dt := timestamp.Sub(s.LastUpdate).Seconds()
	if dt <= 0 {
		return
	}

	// 动态调整平滑因子（数据间隔越长，平滑因子越大）
	alpha := 1 - math.Pow(1-s.Alpha, dt)
	beta := 1 - math.Pow(1-s.Beta, dt)
	gamma := 1 - math.Pow(1-s.Gamma, dt)

	// 保存旧值
	oldValue := s.Value

	// 更新EWMA值
	s.Value = alpha*value + (1-alpha)*s.Value

	// 更新趋势（变化率）
	newTrend := (s.Value - oldValue) / dt
	s.Trend = beta*newTrend + (1-beta)*s.Trend

	// 更新方差估计
	deviation := math.Abs(value - s.Value)
	s.Variance = gamma*deviation*deviation + (1-gamma)*s.Variance

	s.LastUpdate = timestamp
}

// calculateConfidence 计算置信度
func (lp *LatencyPredictor) calculateConfidence(state *EWMAState, deltaT float64) float64 {
	if !state.Initialized {
		return 0
	}

	// 基于方差的置信度
	stdDev := math.Sqrt(state.Variance)
	varianceConfidence := math.Exp(-stdDev / 50.0) // 标准差50ms时置信度约0.37

	// 基于时间的置信度衰减
	timeConfidence := math.Exp(-deltaT / 20.0) // 20秒半衰期

	// 综合置信度
	confidence := varianceConfidence * timeConfidence

	return math.Max(0, math.Min(1, confidence))
}

// UpdateWithActual 用实际值更新（在线学习）
func (lp *LatencyPredictor) UpdateWithActual(predicted, actual float64) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	error := math.Abs(predicted - actual)
	lp.errorSum += error
	lp.errorCount++
}

// GetMAE 获取平均绝对误差
func (lp *LatencyPredictor) GetMAE() float64 {
	lp.mu.RLock()
	defer lp.mu.RUnlock()

	if lp.errorCount == 0 {
		return 0
	}
	return lp.errorSum / float64(lp.errorCount)
}

// GetStats 获取统计信息
func (lp *LatencyPredictor) GetStats() map[string]interface{} {
	lp.mu.RLock()
	defer lp.mu.RUnlock()

	return map[string]interface{}{
		"prediction_count": lp.predictionCount,
		"mae_ms":           lp.GetMAE(),
		"state_count":      len(lp.ewmaStates),
	}
}

// GetNodeState 获取节点的EWMA状态（用于调试）
func (lp *LatencyPredictor) GetNodeState(nodeName string) map[string]interface{} {
	lp.mu.RLock()
	defer lp.mu.RUnlock()

	state, exists := lp.ewmaStates[nodeName]
	if !exists {
		return nil
	}

	return map[string]interface{}{
		"value":       state.Value,
		"trend":       state.Trend,
		"variance":    state.Variance,
		"std_dev":     math.Sqrt(state.Variance),
		"last_update": state.LastUpdate,
		"initialized": state.Initialized,
	}
}

// Reset 重置所有状态
func (lp *LatencyPredictor) Reset() {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	lp.ewmaStates = make(map[string]*EWMAState)
	lp.predictionCount = 0
	lp.errorSum = 0
	lp.errorCount = 0
}

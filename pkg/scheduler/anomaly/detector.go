package anomaly

import (
	"log"
	"math"
	"sync"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// AnomalyDetector 异常检测器主入口
// 整合多种检测方法：统计方法、Isolation Forest、规则检测
type AnomalyDetector struct {
	config *DetectorConfig

	// 子检测器
	statisticalDetector *StatisticalDetector
	isolationForest     *IsolationForest
	ruleBasedDetector   *RuleBasedDetector

	// 节点状态
	nodeStates map[string]*NodeAnomalyState
	mu         sync.RWMutex

	// 统计
	stats *DetectorStats

	// 异常历史
	anomalyHistory []*Anomaly
	historyMaxSize int

	// 日志
	verbose bool
}

// NewAnomalyDetector 创建异常检测器
func NewAnomalyDetector(config *DetectorConfig) *AnomalyDetector {
	if config == nil {
		config = DefaultConfig()
	}

	ad := &AnomalyDetector{
		config:         config,
		nodeStates:     make(map[string]*NodeAnomalyState),
		stats:          &DetectorStats{AnomaliesByType: make(map[AnomalyType]int64), AnomaliesBySeverity: make(map[AnomalySeverity]int64)},
		historyMaxSize: 1000,
		verbose:        false,
	}

	// 初始化子检测器
	if config.EnableStatistical {
		ad.statisticalDetector = NewStatisticalDetector(config)
	}
	if config.EnableIsolationForest {
		ad.isolationForest = NewIsolationForest(config)
	}
	if config.EnableRuleBased {
		ad.ruleBasedDetector = NewRuleBasedDetector(config)
	}

	return ad
}

// SetVerbose 设置详细日志
func (ad *AnomalyDetector) SetVerbose(verbose bool) {
	ad.verbose = verbose
}

// Detect 检测单个节点的异常
func (ad *AnomalyDetector) Detect(metrics *models.UAVMetrics) []*Anomaly {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	var allAnomalies []*Anomaly
	nodeName := metrics.NodeName

	// 更新统计
	ad.stats.TotalChecks++
	ad.stats.LastCheckTime = time.Now()

	// 1. 规则检测
	if ad.ruleBasedDetector != nil {
		ruleAnomalies := ad.ruleBasedDetector.Detect(metrics)
		allAnomalies = append(allAnomalies, ruleAnomalies...)
	}

	// 2. 统计检测
	if ad.statisticalDetector != nil {
		statAnomalies := ad.detectStatistical(metrics)
		allAnomalies = append(allAnomalies, statAnomalies...)
	}

	// 3. Isolation Forest检测
	if ad.isolationForest != nil {
		ifAnomaly := ad.detectIsolationForest(metrics)
		if ifAnomaly != nil {
			allAnomalies = append(allAnomalies, ifAnomaly)
		}
	}

	// 去重和合并
	allAnomalies = ad.deduplicateAnomalies(allAnomalies)

	// 更新统计
	ad.stats.AnomaliesFound += int64(len(allAnomalies))
	for _, a := range allAnomalies {
		ad.stats.AnomaliesByType[a.Type]++
		ad.stats.AnomaliesBySeverity[a.Severity]++
	}

	// 更新节点状态
	ad.updateNodeState(nodeName, allAnomalies)

	// 添加到历史
	ad.addToHistory(allAnomalies)

	// 触发回调
	if ad.config.OnAnomalyDetected != nil && len(allAnomalies) > 0 {
		for _, anomaly := range allAnomalies {
			ad.config.OnAnomalyDetected(&AnomalyEvent{
				Anomaly:   anomaly,
				Action:    ad.determineAction(anomaly),
				Timestamp: time.Now(),
			})
		}
	}

	// 日志
	if ad.verbose && len(allAnomalies) > 0 {
		log.Printf("[AnomalyDetector] Node=%s detected %d anomalies", nodeName, len(allAnomalies))
		for _, a := range allAnomalies {
			log.Printf("  - [%s] %s: %s (score=%.2f)", a.Severity, a.Type, a.Message, a.Score)
		}
	}

	return allAnomalies
}

// detectStatistical 执行统计检测
func (ad *AnomalyDetector) detectStatistical(metrics *models.UAVMetrics) []*Anomaly {
	var anomalies []*Anomaly
	nodeName := metrics.NodeName

	// 检测电池
	if a := ad.statisticalDetector.CheckMetric(nodeName, "battery", metrics.Battery.RemainingPercent); a != nil {
		a.MetricsSnapshot = metrics
		anomalies = append(anomalies, a)
	}

	// 检测网络延迟
	if metrics.Network != nil {
		if a := ad.statisticalDetector.CheckMetric(nodeName, "latency", metrics.Network.Latency); a != nil {
			a.MetricsSnapshot = metrics
			anomalies = append(anomalies, a)
		}
	}

	// 检测CPU
	if metrics.Performance != nil {
		if a := ad.statisticalDetector.CheckMetric(nodeName, "cpu", metrics.Performance.CPUUsage); a != nil {
			a.MetricsSnapshot = metrics
			anomalies = append(anomalies, a)
		}
		if a := ad.statisticalDetector.CheckMetric(nodeName, "memory", metrics.Performance.MemoryUsage); a != nil {
			a.MetricsSnapshot = metrics
			anomalies = append(anomalies, a)
		}
	}

	return anomalies
}

// detectIsolationForest 执行Isolation Forest检测
func (ad *AnomalyDetector) detectIsolationForest(metrics *models.UAVMetrics) *Anomaly {
	// 构建特征向量
	features := ad.buildFeatureVector(metrics)

	anomaly := ad.isolationForest.Detect(metrics.NodeName, features)
	if anomaly != nil {
		anomaly.MetricsSnapshot = metrics
	}

	return anomaly
}

// buildFeatureVector 从metrics构建特征向量
func (ad *AnomalyDetector) buildFeatureVector(metrics *models.UAVMetrics) []float64 {
	features := make([]float64, 6)

	// 1. 电量
	features[0] = metrics.Battery.RemainingPercent / 100.0

	// 2. 电量变化率（归一化）
	features[1] = 0 // 需要历史数据计算

	// 3. 位置变化（归一化）
	features[2] = 0 // 需要历史数据计算

	// 4. 网络延迟（归一化）
	if metrics.Network != nil {
		features[3] = math.Min(metrics.Network.Latency/1000.0, 1.0)
	}

	// 5. CPU使用率
	if metrics.Performance != nil {
		features[4] = metrics.Performance.CPUUsage / 100.0
	}

	// 6. 内存使用率
	if metrics.Performance != nil {
		features[5] = metrics.Performance.MemoryUsage / 100.0
	}

	return features
}

// deduplicateAnomalies 去重异常
func (ad *AnomalyDetector) deduplicateAnomalies(anomalies []*Anomaly) []*Anomaly {
	if len(anomalies) <= 1 {
		return anomalies
	}

	seen := make(map[AnomalyType]bool)
	result := make([]*Anomaly, 0, len(anomalies))

	for _, a := range anomalies {
		// 保留每种类型中分数最高的
		if !seen[a.Type] {
			seen[a.Type] = true
			result = append(result, a)
		}
	}

	return result
}

// updateNodeState 更新节点异常状态
func (ad *AnomalyDetector) updateNodeState(nodeName string, anomalies []*Anomaly) {
	state, exists := ad.nodeStates[nodeName]
	if !exists {
		state = &NodeAnomalyState{
			NodeName:      nodeName,
			HealthHistory: make([]bool, 0, 10),
		}
		ad.nodeStates[nodeName] = state
	}

	// 更新活跃异常
	state.ActiveAnomalies = anomalies
	state.LastChecked = time.Now()

	// 计算综合异常分数
	if len(anomalies) == 0 {
		state.AnomalyScore = 0
		state.IsHealthy = true
	} else {
		var maxScore float64
		for _, a := range anomalies {
			if a.Score > maxScore {
				maxScore = a.Score
			}
		}
		state.AnomalyScore = maxScore
		state.IsHealthy = maxScore < 0.5
	}

	// 更新健康历史
	state.HealthHistory = append(state.HealthHistory, state.IsHealthy)
	if len(state.HealthHistory) > 10 {
		state.HealthHistory = state.HealthHistory[1:]
	}
}

// addToHistory 添加到异常历史
func (ad *AnomalyDetector) addToHistory(anomalies []*Anomaly) {
	ad.anomalyHistory = append(ad.anomalyHistory, anomalies...)
	if len(ad.anomalyHistory) > ad.historyMaxSize {
		ad.anomalyHistory = ad.anomalyHistory[len(ad.anomalyHistory)-ad.historyMaxSize:]
	}
}

// determineAction 确定异常处理动作
func (ad *AnomalyDetector) determineAction(anomaly *Anomaly) AnomalyAction {
	switch anomaly.Severity {
	case SeverityFatal:
		return ActionQuarantine
	case SeverityCritical:
		return ActionEvict
	case SeverityWarning:
		return ActionAlert
	default:
		return ActionLog
	}
}

// DetectBatch 批量检测
func (ad *AnomalyDetector) DetectBatch(metricsList []*models.UAVMetrics) map[string][]*Anomaly {
	result := make(map[string][]*Anomaly)
	for _, metrics := range metricsList {
		anomalies := ad.Detect(metrics)
		if len(anomalies) > 0 {
			result[metrics.NodeName] = anomalies
		}
	}
	return result
}

// GetNodeState 获取节点异常状态
func (ad *AnomalyDetector) GetNodeState(nodeName string) *NodeAnomalyState {
	ad.mu.RLock()
	defer ad.mu.RUnlock()
	return ad.nodeStates[nodeName]
}

// GetAllNodeStates 获取所有节点状态
func (ad *AnomalyDetector) GetAllNodeStates() map[string]*NodeAnomalyState {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	// 返回副本
	result := make(map[string]*NodeAnomalyState)
	for k, v := range ad.nodeStates {
		result[k] = v
	}
	return result
}

// GetHealthyNodes 获取健康节点列表
func (ad *AnomalyDetector) GetHealthyNodes() []string {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	var healthy []string
	for nodeName, state := range ad.nodeStates {
		if state.IsHealthy {
			healthy = append(healthy, nodeName)
		}
	}
	return healthy
}

// GetUnhealthyNodes 获取不健康节点列表
func (ad *AnomalyDetector) GetUnhealthyNodes() []string {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	var unhealthy []string
	for nodeName, state := range ad.nodeStates {
		if !state.IsHealthy {
			unhealthy = append(unhealthy, nodeName)
		}
	}
	return unhealthy
}

// GetStats 获取统计信息
func (ad *AnomalyDetector) GetStats() *DetectorStats {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	// 返回副本
	stats := *ad.stats
	stats.NodesMonitored = len(ad.nodeStates)
	stats.AnomaliesByType = make(map[AnomalyType]int64)
	stats.AnomaliesBySeverity = make(map[AnomalySeverity]int64)
	for k, v := range ad.stats.AnomaliesByType {
		stats.AnomaliesByType[k] = v
	}
	for k, v := range ad.stats.AnomaliesBySeverity {
		stats.AnomaliesBySeverity[k] = v
	}
	return &stats
}

// GetDetailedStats 获取详细统计
func (ad *AnomalyDetector) GetDetailedStats() map[string]interface{} {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	result := map[string]interface{}{
		"total_checks":     ad.stats.TotalChecks,
		"anomalies_found":  ad.stats.AnomaliesFound,
		"nodes_monitored":  len(ad.nodeStates),
		"last_check_time":  ad.stats.LastCheckTime,
		"by_type":          ad.stats.AnomaliesByType,
		"by_severity":      ad.stats.AnomaliesBySeverity,
		"history_size":     len(ad.anomalyHistory),
	}

	// 子检测器状态
	if ad.isolationForest != nil {
		result["isolation_forest"] = ad.isolationForest.GetStats()
	}

	return result
}

// GetAnomalyHistory 获取异常历史
func (ad *AnomalyDetector) GetAnomalyHistory(limit int) []*Anomaly {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	if limit <= 0 || limit > len(ad.anomalyHistory) {
		limit = len(ad.anomalyHistory)
	}

	// 返回最近的
	start := len(ad.anomalyHistory) - limit
	result := make([]*Anomaly, limit)
	copy(result, ad.anomalyHistory[start:])
	return result
}

// Reset 重置检测器
func (ad *AnomalyDetector) Reset() {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	ad.nodeStates = make(map[string]*NodeAnomalyState)
	ad.stats = &DetectorStats{
		AnomaliesByType:     make(map[AnomalyType]int64),
		AnomaliesBySeverity: make(map[AnomalySeverity]int64),
	}
	ad.anomalyHistory = nil

	if ad.statisticalDetector != nil {
		ad.statisticalDetector.Reset()
	}
	if ad.isolationForest != nil {
		ad.isolationForest.Reset()
	}
	if ad.ruleBasedDetector != nil {
		ad.ruleBasedDetector.Reset()
	}
}

// FilterHealthyMetrics 过滤掉不健康节点的metrics
func (ad *AnomalyDetector) FilterHealthyMetrics(metricsList []*models.UAVMetrics) []*models.UAVMetrics {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	var healthy []*models.UAVMetrics
	for _, metrics := range metricsList {
		state := ad.nodeStates[metrics.NodeName]
		if state == nil || state.IsHealthy {
			healthy = append(healthy, metrics)
		}
	}
	return healthy
}

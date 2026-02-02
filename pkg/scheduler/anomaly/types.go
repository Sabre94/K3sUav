package anomaly

import (
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// AnomalyType 异常类型
type AnomalyType string

const (
	// 电池相关异常
	AnomalyBatteryDrop     AnomalyType = "battery_drop"      // 电量骤降
	AnomalyBatterySpike    AnomalyType = "battery_spike"     // 电量异常上升（虚电）
	AnomalyBatteryLow      AnomalyType = "battery_low"       // 电量过低
	AnomalyBatteryCritical AnomalyType = "battery_critical"  // 电量危急

	// 位置相关异常
	AnomalyPositionJump  AnomalyType = "position_jump"  // 位置突变（GPS漂移）
	AnomalyPositionStuck AnomalyType = "position_stuck" // 位置卡住（可能故障）
	AnomalyAltitudeSpike AnomalyType = "altitude_spike" // 高度异常

	// 网络相关异常
	AnomalyLatencySpike  AnomalyType = "latency_spike"  // 延迟突增
	AnomalyPacketLoss    AnomalyType = "packet_loss"    // 丢包率高
	AnomalyNetworkDown   AnomalyType = "network_down"   // 网络中断

	// 性能相关异常
	AnomalyCPUHigh       AnomalyType = "cpu_high"        // CPU过高
	AnomalyMemoryHigh    AnomalyType = "memory_high"     // 内存过高
	AnomalyTemperatureHigh AnomalyType = "temperature_high" // 温度过高

	// 综合异常
	AnomalyMultiple AnomalyType = "multiple" // 多重异常
	AnomalyUnknown  AnomalyType = "unknown"  // 未知异常
)

// AnomalySeverity 异常严重程度
type AnomalySeverity string

const (
	SeverityInfo     AnomalySeverity = "info"     // 信息，无需处理
	SeverityWarning  AnomalySeverity = "warning"  // 警告，需要关注
	SeverityCritical AnomalySeverity = "critical" // 严重，需要立即处理
	SeverityFatal    AnomalySeverity = "fatal"    // 致命，节点不可用
)

// Anomaly 异常记录
type Anomaly struct {
	ID          string          `json:"id"`          // 异常ID
	NodeName    string          `json:"nodeName"`    // 节点名称
	Type        AnomalyType     `json:"type"`        // 异常类型
	Severity    AnomalySeverity `json:"severity"`    // 严重程度
	Score       float64         `json:"score"`       // 异常分数 (0-1, 越高越异常)
	Message     string          `json:"message"`     // 异常描述
	DetectedAt  time.Time       `json:"detectedAt"`  // 检测时间
	DetectedBy  string          `json:"detectedBy"`  // 检测器名称

	// 上下文信息
	CurrentValue  float64 `json:"currentValue,omitempty"`  // 当前值
	ExpectedValue float64 `json:"expectedValue,omitempty"` // 预期值
	Threshold     float64 `json:"threshold,omitempty"`     // 阈值

	// 原始数据快照
	MetricsSnapshot *models.UAVMetrics `json:"metricsSnapshot,omitempty"`
}

// AnomalyEvent 异常事件（用于回调/通知）
type AnomalyEvent struct {
	Anomaly   *Anomaly
	Action    AnomalyAction
	Timestamp time.Time
}

// AnomalyAction 异常处理动作
type AnomalyAction string

const (
	ActionNone      AnomalyAction = "none"       // 无动作
	ActionLog       AnomalyAction = "log"        // 记录日志
	ActionAlert     AnomalyAction = "alert"      // 发送告警
	ActionEvict     AnomalyAction = "evict"      // 驱逐Pod
	ActionQuarantine AnomalyAction = "quarantine" // 隔离节点
)

// DetectorConfig 检测器配置
type DetectorConfig struct {
	// 启用的检测器
	EnableStatistical    bool `json:"enableStatistical"`
	EnableIsolationForest bool `json:"enableIsolationForest"`
	EnableRuleBased      bool `json:"enableRuleBased"`

	// 统计检测器配置
	StatisticalWindowSize int     `json:"statisticalWindowSize"` // 滑动窗口大小
	ZScoreThreshold       float64 `json:"zScoreThreshold"`       // Z-score阈值

	// Isolation Forest配置
	IFNumTrees     int     `json:"ifNumTrees"`     // 树的数量
	IFSampleSize   int     `json:"ifSampleSize"`   // 采样大小
	IFThreshold    float64 `json:"ifThreshold"`    // 异常阈值

	// 规则检测器配置
	BatteryDropThreshold   float64 `json:"batteryDropThreshold"`   // 电量骤降阈值 (%/s)
	BatteryLowThreshold    float64 `json:"batteryLowThreshold"`    // 低电量阈值 (%)
	BatteryCriticalThreshold float64 `json:"batteryCriticalThreshold"` // 危急电量阈值 (%)
	PositionJumpThreshold  float64 `json:"positionJumpThreshold"`  // 位置突变阈值 (米)
	LatencySpikeThreshold  float64 `json:"latencySpikeThreshold"`  // 延迟突增阈值 (ms)
	CPUHighThreshold       float64 `json:"cpuHighThreshold"`       // CPU高阈值 (%)
	MemoryHighThreshold    float64 `json:"memoryHighThreshold"`    // 内存高阈值 (%)
	TemperatureHighThreshold float64 `json:"temperatureHighThreshold"` // 温度高阈值 (°C)

	// 回调配置
	OnAnomalyDetected func(*AnomalyEvent) `json:"-"` // 异常检测回调
}

// DefaultConfig 返回默认配置
func DefaultConfig() *DetectorConfig {
	return &DetectorConfig{
		EnableStatistical:     true,
		EnableIsolationForest: true,
		EnableRuleBased:       true,

		StatisticalWindowSize: 30,
		ZScoreThreshold:       3.0,

		IFNumTrees:   100,
		IFSampleSize: 256,
		IFThreshold:  0.6,

		BatteryDropThreshold:     5.0,   // 5%/s 认为是骤降
		BatteryLowThreshold:      20.0,  // 20% 低电量
		BatteryCriticalThreshold: 10.0,  // 10% 危急
		PositionJumpThreshold:    100.0, // 100米突变
		LatencySpikeThreshold:    500.0, // 500ms延迟突增
		CPUHighThreshold:         90.0,  // 90% CPU
		MemoryHighThreshold:      90.0,  // 90% 内存
		TemperatureHighThreshold: 70.0,  // 70°C 温度
	}
}

// DetectorStats 检测器统计信息
type DetectorStats struct {
	TotalChecks      int64            `json:"totalChecks"`
	AnomaliesFound   int64            `json:"anomaliesFound"`
	AnomaliesByType  map[AnomalyType]int64 `json:"anomaliesByType"`
	AnomaliesBySeverity map[AnomalySeverity]int64 `json:"anomaliesBySeverity"`
	LastCheckTime    time.Time        `json:"lastCheckTime"`
	NodesMonitored   int              `json:"nodesMonitored"`
}

// NodeAnomalyState 节点异常状态
type NodeAnomalyState struct {
	NodeName       string      `json:"nodeName"`
	IsHealthy      bool        `json:"isHealthy"`
	ActiveAnomalies []*Anomaly `json:"activeAnomalies"`
	AnomalyScore   float64     `json:"anomalyScore"`    // 综合异常分数
	LastChecked    time.Time   `json:"lastChecked"`
	HealthHistory  []bool      `json:"healthHistory"`   // 最近N次检查结果
}

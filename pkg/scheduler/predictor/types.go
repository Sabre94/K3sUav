package predictor

import (
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// PredictedMetrics 预测增强的UAVMetrics
type PredictedMetrics struct {
	*models.UAVMetrics

	// 预测值
	PredictedBattery  float64        `json:"predictedBattery"`  // 预测电量 (%)
	PredictedPosition *Position      `json:"predictedPosition"` // 预测位置
	PredictedLatency  float64        `json:"predictedLatency"`  // 预测延迟 (ms)

	// 数据元信息
	DataAge        time.Duration `json:"dataAge"`        // 数据年龄（距上次更新）
	PredictionTime time.Time     `json:"predictionTime"` // 预测执行时刻

	// 置信度 (0-1)，随数据年龄衰减
	BatteryConfidence  float64 `json:"batteryConfidence"`
	PositionConfidence float64 `json:"positionConfidence"`
	LatencyConfidence  float64 `json:"latencyConfidence"`

	// 是否使用了预测（如果数据足够新鲜，可能直接用原值）
	UsedPrediction bool `json:"usedPrediction"`
}

// Position 三维位置
type Position struct {
	X float64 `json:"x"` // X坐标 (米)
	Y float64 `json:"y"` // Y坐标 (米)
	Z float64 `json:"z"` // Z坐标/高度 (米)
}

// Velocity 三维速度
type Velocity struct {
	Vx float64 `json:"vx"` // X方向速度 (m/s)
	Vy float64 `json:"vy"` // Y方向速度 (m/s)
	Vz float64 `json:"vz"` // Z方向速度 (m/s)
}

// HistoryPoint 历史数据点
type HistoryPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Battery   float64   `json:"battery"`  // 电量 (%)
	Position  Position  `json:"position"` // 位置
	Velocity  Velocity  `json:"velocity"` // 速度
	Latency   float64   `json:"latency"`  // 网络延迟 (ms)
	Speed     float64   `json:"speed"`    // 飞行速度 (m/s)
}

// PredictorConfig 预测器配置
type PredictorConfig struct {
	// 历史数据配置
	HistorySize int           `json:"historySize"` // 每个节点保留的历史点数
	MaxDataAge  time.Duration `json:"maxDataAge"`  // 超过此年龄的数据不使用预测

	// 置信度半衰期
	BatteryConfidenceHalfLife  time.Duration `json:"batteryConfidenceHalfLife"`
	PositionConfidenceHalfLife time.Duration `json:"positionConfidenceHalfLife"`
	LatencyConfidenceHalfLife  time.Duration `json:"latencyConfidenceHalfLife"`

	// 预测阈值：数据年龄超过此值才启用预测
	PredictionThreshold time.Duration `json:"predictionThreshold"`

	// LSTM配置
	LSTMEnabled    bool   `json:"lstmEnabled"`
	LSTMModelPath  string `json:"lstmModelPath"`
	LSTMHiddenSize int    `json:"lstmHiddenSize"`
	LSTMSeqLength  int    `json:"lstmSeqLength"`

	// 卡尔曼滤波配置
	KalmanProcessNoise     float64 `json:"kalmanProcessNoise"`
	KalmanMeasurementNoise float64 `json:"kalmanMeasurementNoise"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *PredictorConfig {
	return &PredictorConfig{
		HistorySize: 50,                     // 保留50个历史点
		MaxDataAge:  5 * time.Minute,        // 5分钟以上的数据不预测

		BatteryConfidenceHalfLife:  30 * time.Second, // 电量预测30秒半衰期
		PositionConfidenceHalfLife: 10 * time.Second, // 位置预测10秒半衰期
		LatencyConfidenceHalfLife:  20 * time.Second, // 延迟预测20秒半衰期

		PredictionThreshold: 2 * time.Second, // 2秒内的数据直接用原值

		LSTMEnabled:    true,
		LSTMHiddenSize: 32,
		LSTMSeqLength:  10,

		KalmanProcessNoise:     0.1,
		KalmanMeasurementNoise: 1.0,
	}
}

// PredictionStats 预测统计信息
type PredictionStats struct {
	TotalPredictions   int64   `json:"totalPredictions"`
	BatteryPredictions int64   `json:"batteryPredictions"`
	PositionPredictions int64  `json:"positionPredictions"`
	LatencyPredictions int64   `json:"latencyPredictions"`

	// 预测误差统计（在线学习时更新）
	BatteryMAE  float64 `json:"batteryMAE"`  // 平均绝对误差
	PositionMAE float64 `json:"positionMAE"`
	LatencyMAE  float64 `json:"latencyMAE"`
}

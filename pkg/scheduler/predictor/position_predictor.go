package predictor

import (
	"math"
	"sync"
	"time"
)

// PositionPredictor 位置预测器
// 使用卡尔曼滤波 + 速度外推
type PositionPredictor struct {
	config  *PredictorConfig
	filters map[string]*KalmanFilter // 每个节点一个滤波器
	mu      sync.RWMutex

	// 统计
	predictionCount int64
	errorSum        float64
	errorCount      int64
}

// NewPositionPredictor 创建位置预测器
func NewPositionPredictor(config *PredictorConfig) *PositionPredictor {
	return &PositionPredictor{
		config:  config,
		filters: make(map[string]*KalmanFilter),
	}
}

// Predict 预测指定时间的位置
func (pp *PositionPredictor) Predict(history *HistoryBuffer, targetTime time.Time) (*Position, float64) {
	pp.mu.Lock()
	pp.predictionCount++
	pp.mu.Unlock()

	// 获取或创建卡尔曼滤波器
	filter := pp.getOrCreateFilter(history.NodeName)

	// 获取历史数据
	points := history.GetLastN(10)
	if len(points) == 0 {
		return nil, 0
	}

	latest := points[len(points)-1]
	deltaT := targetTime.Sub(latest.Timestamp).Seconds()

	// 如果是预测过去或当前
	if deltaT <= 0 {
		return &latest.Position, 1.0
	}

	// 更新卡尔曼滤波器状态
	for _, point := range points {
		filter.Update(point.Position, point.Velocity, point.Timestamp)
	}

	// 预测
	predicted, confidence := filter.Predict(deltaT)

	return predicted, confidence
}

// PredictSimple 简单速度外推预测（不使用卡尔曼滤波）
func (pp *PositionPredictor) PredictSimple(history *HistoryBuffer, targetTime time.Time) (*Position, float64) {
	points := history.GetLastN(5)
	if len(points) == 0 {
		return nil, 0
	}

	latest := points[len(points)-1]
	deltaT := targetTime.Sub(latest.Timestamp).Seconds()

	if deltaT <= 0 {
		return &latest.Position, 1.0
	}

	// 计算平均速度
	var avgVx, avgVy, avgVz float64
	var count int

	for i := 1; i < len(points); i++ {
		dt := points[i].Timestamp.Sub(points[i-1].Timestamp).Seconds()
		if dt <= 0 {
			continue
		}

		// 使用速度数据（如果有）
		if points[i].Velocity.Vx != 0 || points[i].Velocity.Vy != 0 || points[i].Velocity.Vz != 0 {
			avgVx += points[i].Velocity.Vx
			avgVy += points[i].Velocity.Vy
			avgVz += points[i].Velocity.Vz
		} else {
			// 否则从位置差计算
			avgVx += (points[i].Position.X - points[i-1].Position.X) / dt
			avgVy += (points[i].Position.Y - points[i-1].Position.Y) / dt
			avgVz += (points[i].Position.Z - points[i-1].Position.Z) / dt
		}
		count++
	}

	if count == 0 {
		return &latest.Position, 0.5
	}

	avgVx /= float64(count)
	avgVy /= float64(count)
	avgVz /= float64(count)

	// 预测位置
	predicted := &Position{
		X: latest.Position.X + avgVx*deltaT,
		Y: latest.Position.Y + avgVy*deltaT,
		Z: latest.Position.Z + avgVz*deltaT,
	}

	// 置信度随时间衰减
	confidence := math.Exp(-deltaT / 10.0) // 10秒半衰期

	return predicted, confidence
}

// getOrCreateFilter 获取或创建节点的卡尔曼滤波器
func (pp *PositionPredictor) getOrCreateFilter(nodeName string) *KalmanFilter {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	if filter, exists := pp.filters[nodeName]; exists {
		return filter
	}

	filter := NewKalmanFilter(pp.config)
	pp.filters[nodeName] = filter
	return filter
}

// UpdateWithActual 用实际值更新（在线学习）
func (pp *PositionPredictor) UpdateWithActual(nodeName string, predicted, actual *Position) {
	if predicted == nil || actual == nil {
		return
	}

	pp.mu.Lock()
	defer pp.mu.Unlock()

	// 计算欧氏距离误差
	error := math.Sqrt(
		math.Pow(predicted.X-actual.X, 2) +
			math.Pow(predicted.Y-actual.Y, 2) +
			math.Pow(predicted.Z-actual.Z, 2),
	)

	pp.errorSum += error
	pp.errorCount++
}

// GetMAE 获取平均绝对误差（米）
func (pp *PositionPredictor) GetMAE() float64 {
	pp.mu.RLock()
	defer pp.mu.RUnlock()

	if pp.errorCount == 0 {
		return 0
	}
	return pp.errorSum / float64(pp.errorCount)
}

// GetStats 获取统计信息
func (pp *PositionPredictor) GetStats() map[string]interface{} {
	pp.mu.RLock()
	defer pp.mu.RUnlock()

	return map[string]interface{}{
		"prediction_count": pp.predictionCount,
		"mae_meters":       pp.GetMAE(),
		"filter_count":     len(pp.filters),
	}
}

// KalmanFilter 卡尔曼滤波器
// 状态向量: [x, y, z, vx, vy, vz]
type KalmanFilter struct {
	config *PredictorConfig

	// 状态向量 [x, y, z, vx, vy, vz]
	state []float64

	// 协方差矩阵 (6x6)
	P [][]float64

	// 过程噪声
	Q float64

	// 测量噪声
	R float64

	// 上次更新时间
	lastUpdate time.Time
	initialized bool
}

// NewKalmanFilter 创建卡尔曼滤波器
func NewKalmanFilter(config *PredictorConfig) *KalmanFilter {
	kf := &KalmanFilter{
		config: config,
		state:  make([]float64, 6),
		P:      make([][]float64, 6),
		Q:      config.KalmanProcessNoise,
		R:      config.KalmanMeasurementNoise,
	}

	// 初始化协方差矩阵为单位矩阵
	for i := range kf.P {
		kf.P[i] = make([]float64, 6)
		kf.P[i][i] = 1.0
	}

	return kf
}

// Update 用新测量值更新滤波器
func (kf *KalmanFilter) Update(pos Position, vel Velocity, timestamp time.Time) {
	if !kf.initialized {
		// 首次初始化
		kf.state[0] = pos.X
		kf.state[1] = pos.Y
		kf.state[2] = pos.Z
		kf.state[3] = vel.Vx
		kf.state[4] = vel.Vy
		kf.state[5] = vel.Vz
		kf.lastUpdate = timestamp
		kf.initialized = true
		return
	}

	// 计算时间差
	dt := timestamp.Sub(kf.lastUpdate).Seconds()
	if dt <= 0 {
		return
	}

	// 预测步骤
	kf.predictStep(dt)

	// 更新步骤
	measurement := []float64{pos.X, pos.Y, pos.Z, vel.Vx, vel.Vy, vel.Vz}
	kf.updateStep(measurement)

	kf.lastUpdate = timestamp
}

// predictStep 预测步骤
func (kf *KalmanFilter) predictStep(dt float64) {
	// 状态转移矩阵 F
	// [1 0 0 dt 0  0 ]
	// [0 1 0 0  dt 0 ]
	// [0 0 1 0  0  dt]
	// [0 0 0 1  0  0 ]
	// [0 0 0 0  1  0 ]
	// [0 0 0 0  0  1 ]

	// 预测状态: x' = F * x
	newState := make([]float64, 6)
	newState[0] = kf.state[0] + kf.state[3]*dt // x + vx*dt
	newState[1] = kf.state[1] + kf.state[4]*dt // y + vy*dt
	newState[2] = kf.state[2] + kf.state[5]*dt // z + vz*dt
	newState[3] = kf.state[3]                   // vx不变
	newState[4] = kf.state[4]                   // vy不变
	newState[5] = kf.state[5]                   // vz不变
	kf.state = newState

	// 预测协方差: P' = F * P * F' + Q
	// 简化处理：增加过程噪声
	for i := 0; i < 6; i++ {
		kf.P[i][i] += kf.Q * dt
	}
}

// updateStep 更新步骤
func (kf *KalmanFilter) updateStep(measurement []float64) {
	// 测量矩阵 H = I (直接观测所有状态)
	// 卡尔曼增益 K = P * H' * (H * P * H' + R)^-1
	// 简化：K = P / (P + R)

	for i := 0; i < 6; i++ {
		// 卡尔曼增益
		k := kf.P[i][i] / (kf.P[i][i] + kf.R)

		// 更新状态
		kf.state[i] = kf.state[i] + k*(measurement[i]-kf.state[i])

		// 更新协方差
		kf.P[i][i] = (1 - k) * kf.P[i][i]
	}
}

// Predict 预测未来位置
func (kf *KalmanFilter) Predict(deltaT float64) (*Position, float64) {
	if !kf.initialized {
		return nil, 0
	}

	// 简单线性外推
	predicted := &Position{
		X: kf.state[0] + kf.state[3]*deltaT,
		Y: kf.state[1] + kf.state[4]*deltaT,
		Z: kf.state[2] + kf.state[5]*deltaT,
	}

	// 置信度基于协方差和预测时间
	posVariance := (kf.P[0][0] + kf.P[1][1] + kf.P[2][2]) / 3
	// 预测越远，不确定性越大
	predictedVariance := posVariance + kf.Q*deltaT*deltaT

	// 映射到置信度
	confidence := math.Exp(-predictedVariance / 100.0) // 归一化因子
	confidence = math.Max(0, math.Min(1, confidence))

	return predicted, confidence
}

// GetState 获取当前状态
func (kf *KalmanFilter) GetState() (Position, Velocity) {
	return Position{
			X: kf.state[0],
			Y: kf.state[1],
			Z: kf.state[2],
		}, Velocity{
			Vx: kf.state[3],
			Vy: kf.state[4],
			Vz: kf.state[5],
		}
}

// Reset 重置滤波器
func (kf *KalmanFilter) Reset() {
	kf.initialized = false
	for i := range kf.state {
		kf.state[i] = 0
	}
	for i := range kf.P {
		for j := range kf.P[i] {
			if i == j {
				kf.P[i][j] = 1.0
			} else {
				kf.P[i][j] = 0
			}
		}
	}
}

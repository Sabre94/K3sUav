package predictor

import (
	"math"
	"sync"
	"time"
)

// BatteryPredictor 电池状态预测器
// 支持两种模式：
// 1. 简单线性预测：基于历史消耗速率
// 2. LSTM预测：基于时间序列神经网络
type BatteryPredictor struct {
	config *PredictorConfig
	lstm   *BatteryLSTM
	mu     sync.RWMutex

	// 在线学习统计
	predictionCount int64
	errorSum        float64
	errorCount      int64
}

// NewBatteryPredictor 创建电池预测器
func NewBatteryPredictor(config *PredictorConfig) *BatteryPredictor {
	bp := &BatteryPredictor{
		config: config,
	}

	if config.LSTMEnabled {
		bp.lstm = NewBatteryLSTM(config)
	}

	return bp
}

// Predict 预测指定时间后的电池电量
func (bp *BatteryPredictor) Predict(history *HistoryBuffer, targetTime time.Time) (float64, float64) {
	bp.mu.Lock()
	bp.predictionCount++
	bp.mu.Unlock()

	// 获取历史数据
	points := history.GetLastN(bp.config.LSTMSeqLength)
	if len(points) < 2 {
		// 数据不足，返回最新值
		if latest, ok := history.GetLatest(); ok {
			return latest.Battery, 0.5
		}
		return 0, 0
	}

	latest := points[len(points)-1]
	deltaT := targetTime.Sub(latest.Timestamp).Seconds()

	// 如果是预测过去或当前，直接返回
	if deltaT <= 0 {
		return latest.Battery, 1.0
	}

	// 尝试LSTM预测
	if bp.lstm != nil && len(points) >= bp.config.LSTMSeqLength {
		predicted, confidence := bp.lstm.Predict(points, deltaT)
		if confidence > 0.5 {
			return bp.clampBattery(predicted), confidence
		}
	}

	// 回退到线性预测
	return bp.linearPredict(points, deltaT)
}

// linearPredict 基于历史数据的线性预测
func (bp *BatteryPredictor) linearPredict(points []HistoryPoint, deltaT float64) (float64, float64) {
	if len(points) < 2 {
		return points[0].Battery, 0.5
	}

	// 计算历史消耗速率（考虑速度因素）
	var totalConsumption float64
	var totalTime float64
	var weightSum float64

	for i := 1; i < len(points); i++ {
		dt := points[i].Timestamp.Sub(points[i-1].Timestamp).Seconds()
		if dt <= 0 {
			continue
		}

		consumption := points[i-1].Battery - points[i].Battery
		rate := consumption / dt

		// 使用指数加权，最近的数据权重更高
		weight := math.Pow(0.9, float64(len(points)-1-i))

		// 考虑速度因素：速度越快，权重越高（因为高速时耗电数据更有参考价值）
		avgSpeed := (points[i].Speed + points[i-1].Speed) / 2
		if avgSpeed > 0 {
			weight *= (1 + avgSpeed/20) // 假设最大速度约20m/s
		}

		totalConsumption += rate * weight
		totalTime += dt * weight
		weightSum += weight
	}

	if weightSum == 0 || totalTime == 0 {
		return points[len(points)-1].Battery, 0.5
	}

	// 加权平均消耗速率 (% per second)
	avgRate := totalConsumption / weightSum

	// 预测电量
	latestBattery := points[len(points)-1].Battery
	predicted := latestBattery - avgRate*deltaT

	// 置信度：基于数据量和时间跨度
	dataConfidence := math.Min(float64(len(points))/10.0, 1.0)
	timeConfidence := math.Exp(-deltaT / 60.0) // 60秒半衰期
	confidence := dataConfidence * timeConfidence

	return bp.clampBattery(predicted), confidence
}

// clampBattery 限制电量在有效范围内
func (bp *BatteryPredictor) clampBattery(battery float64) float64 {
	if battery < 0 {
		return 0
	}
	if battery > 100 {
		return 100
	}
	return battery
}

// UpdateWithActual 用实际值更新模型（在线学习）
func (bp *BatteryPredictor) UpdateWithActual(predicted, actual float64) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	error := math.Abs(predicted - actual)
	bp.errorSum += error
	bp.errorCount++

	// 更新LSTM（如果启用）
	if bp.lstm != nil {
		bp.lstm.UpdateWithError(predicted, actual)
	}
}

// GetMAE 获取平均绝对误差
func (bp *BatteryPredictor) GetMAE() float64 {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	if bp.errorCount == 0 {
		return 0
	}
	return bp.errorSum / float64(bp.errorCount)
}

// GetStats 获取预测统计
func (bp *BatteryPredictor) GetStats() map[string]interface{} {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	return map[string]interface{}{
		"prediction_count": bp.predictionCount,
		"mae":              bp.GetMAE(),
		"lstm_enabled":     bp.lstm != nil,
	}
}

// BatteryLSTM 电池预测LSTM模型
// 简化实现：使用单层LSTM + 全连接层
type BatteryLSTM struct {
	config *PredictorConfig

	// LSTM参数
	inputSize  int
	hiddenSize int

	// 权重矩阵（输入门、遗忘门、单元门、输出门）
	Wi, Wf, Wc, Wo [][]float64 // input weights
	Ui, Uf, Uc, Uo [][]float64 // hidden weights
	bi, bf, bc, bo []float64   // biases

	// 输出层
	Wy []float64
	by float64

	// 状态
	h, c []float64 // hidden state, cell state

	// 训练缓冲
	trainingBuffer []trainingExample
	mu             sync.Mutex
}

type trainingExample struct {
	input    [][]float64
	target   float64
	predicted float64
}

// NewBatteryLSTM 创建LSTM模型
func NewBatteryLSTM(config *PredictorConfig) *BatteryLSTM {
	inputSize := 3  // [battery, speed, deltaT]
	hiddenSize := config.LSTMHiddenSize
	if hiddenSize == 0 {
		hiddenSize = 32
	}

	lstm := &BatteryLSTM{
		config:     config,
		inputSize:  inputSize,
		hiddenSize: hiddenSize,
		h:          make([]float64, hiddenSize),
		c:          make([]float64, hiddenSize),
	}

	// 初始化权重（Xavier初始化）
	lstm.initWeights()

	return lstm
}

// initWeights Xavier初始化权重
func (lstm *BatteryLSTM) initWeights() {
	// 使用固定种子保证可重复性
	scale := math.Sqrt(2.0 / float64(lstm.inputSize+lstm.hiddenSize))

	// 初始化输入权重
	lstm.Wi = lstm.randomMatrix(lstm.hiddenSize, lstm.inputSize, scale)
	lstm.Wf = lstm.randomMatrix(lstm.hiddenSize, lstm.inputSize, scale)
	lstm.Wc = lstm.randomMatrix(lstm.hiddenSize, lstm.inputSize, scale)
	lstm.Wo = lstm.randomMatrix(lstm.hiddenSize, lstm.inputSize, scale)

	// 初始化隐藏权重
	lstm.Ui = lstm.randomMatrix(lstm.hiddenSize, lstm.hiddenSize, scale)
	lstm.Uf = lstm.randomMatrix(lstm.hiddenSize, lstm.hiddenSize, scale)
	lstm.Uc = lstm.randomMatrix(lstm.hiddenSize, lstm.hiddenSize, scale)
	lstm.Uo = lstm.randomMatrix(lstm.hiddenSize, lstm.hiddenSize, scale)

	// 初始化偏置
	lstm.bi = make([]float64, lstm.hiddenSize)
	lstm.bf = make([]float64, lstm.hiddenSize)
	lstm.bc = make([]float64, lstm.hiddenSize)
	lstm.bo = make([]float64, lstm.hiddenSize)

	// 遗忘门偏置初始化为1（有助于长期记忆）
	for i := range lstm.bf {
		lstm.bf[i] = 1.0
	}

	// 输出层权重
	lstm.Wy = make([]float64, lstm.hiddenSize)
	for i := range lstm.Wy {
		lstm.Wy[i] = scale * (float64(i%2)*2 - 1) * 0.1
	}
	lstm.by = 0
}

// randomMatrix 创建随机初始化矩阵
func (lstm *BatteryLSTM) randomMatrix(rows, cols int, scale float64) [][]float64 {
	matrix := make([][]float64, rows)
	for i := range matrix {
		matrix[i] = make([]float64, cols)
		for j := range matrix[i] {
			// 简单的伪随机
			matrix[i][j] = scale * (math.Sin(float64(i*cols+j)*12.9898) - 0.5)
		}
	}
	return matrix
}

// Predict 使用LSTM预测电量
func (lstm *BatteryLSTM) Predict(points []HistoryPoint, deltaT float64) (float64, float64) {
	lstm.mu.Lock()
	defer lstm.mu.Unlock()

	if len(points) < 2 {
		return 0, 0
	}

	// 重置状态
	lstm.resetState()

	// 准备输入序列
	sequence := lstm.prepareSequence(points)

	// 前向传播
	for _, input := range sequence {
		lstm.forward(input)
	}

	// 预测消耗速率（% per second）
	consumptionRate := lstm.outputLayer()

	// 计算预测电量
	latestBattery := points[len(points)-1].Battery
	predicted := latestBattery - consumptionRate*deltaT

	// 置信度基于隐藏状态的稳定性
	confidence := lstm.calculateConfidence()

	return predicted, confidence
}

// prepareSequence 准备输入序列
func (lstm *BatteryLSTM) prepareSequence(points []HistoryPoint) [][]float64 {
	sequence := make([][]float64, len(points)-1)

	for i := 1; i < len(points); i++ {
		dt := points[i].Timestamp.Sub(points[i-1].Timestamp).Seconds()
		if dt <= 0 {
			dt = 1.0
		}

		// 归一化输入
		battery := points[i].Battery / 100.0                       // 0-1
		speed := math.Min(points[i].Speed/20.0, 1.0)               // 假设最大20m/s
		deltaTNorm := math.Min(dt/60.0, 1.0)                       // 假设最大60秒间隔

		sequence[i-1] = []float64{battery, speed, deltaTNorm}
	}

	return sequence
}

// forward LSTM前向传播一个时间步
func (lstm *BatteryLSTM) forward(input []float64) {
	// 输入门
	it := lstm.gate(input, lstm.Wi, lstm.Ui, lstm.bi, sigmoid)
	// 遗忘门
	ft := lstm.gate(input, lstm.Wf, lstm.Uf, lstm.bf, sigmoid)
	// 候选单元状态
	ct := lstm.gate(input, lstm.Wc, lstm.Uc, lstm.bc, tanh)
	// 输出门
	ot := lstm.gate(input, lstm.Wo, lstm.Uo, lstm.bo, sigmoid)

	// 更新单元状态
	for i := 0; i < lstm.hiddenSize; i++ {
		lstm.c[i] = ft[i]*lstm.c[i] + it[i]*ct[i]
	}

	// 更新隐藏状态
	for i := 0; i < lstm.hiddenSize; i++ {
		lstm.h[i] = ot[i] * tanh(lstm.c[i])
	}
}

// gate 计算门输出
func (lstm *BatteryLSTM) gate(input []float64, W, U [][]float64, b []float64, activation func(float64) float64) []float64 {
	result := make([]float64, lstm.hiddenSize)

	for i := 0; i < lstm.hiddenSize; i++ {
		sum := b[i]

		// W * input
		for j := 0; j < len(input) && j < len(W[i]); j++ {
			sum += W[i][j] * input[j]
		}

		// U * h
		for j := 0; j < lstm.hiddenSize; j++ {
			sum += U[i][j] * lstm.h[j]
		}

		result[i] = activation(sum)
	}

	return result
}

// outputLayer 输出层
func (lstm *BatteryLSTM) outputLayer() float64 {
	sum := lstm.by
	for i := 0; i < lstm.hiddenSize; i++ {
		sum += lstm.Wy[i] * lstm.h[i]
	}
	// 输出消耗速率，使用ReLU确保非负
	return math.Max(0, sum) * 0.1 // 缩放因子
}

// calculateConfidence 基于隐藏状态计算置信度
func (lstm *BatteryLSTM) calculateConfidence() float64 {
	// 简单方法：基于隐藏状态的平均激活程度
	var sum float64
	for _, h := range lstm.h {
		sum += math.Abs(h)
	}
	avg := sum / float64(lstm.hiddenSize)

	// 映射到0-1，激活程度越高置信度越高
	return math.Min(avg*2, 1.0)
}

// resetState 重置LSTM状态
func (lstm *BatteryLSTM) resetState() {
	for i := range lstm.h {
		lstm.h[i] = 0
		lstm.c[i] = 0
	}
}

// UpdateWithError 根据预测误差更新模型
func (lstm *BatteryLSTM) UpdateWithError(predicted, actual float64) {
	// 简单的在线学习：调整输出层权重
	lstm.mu.Lock()
	defer lstm.mu.Unlock()

	error := actual - predicted
	learningRate := 0.001

	// 更新输出层
	for i := range lstm.Wy {
		lstm.Wy[i] += learningRate * error * lstm.h[i]
	}
	lstm.by += learningRate * error
}

// 激活函数
func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

func tanh(x float64) float64 {
	return math.Tanh(x)
}

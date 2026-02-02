package anomaly

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

// IsolationForest Isolation Forest异常检测算法
// 原理：异常点更容易被隔离（需要更少的分割次数）
type IsolationForest struct {
	config *DetectorConfig
	trees  []*IsolationTree
	mu     sync.RWMutex

	// 训练数据缓存
	trainingBuffer [][]float64
	bufferSize     int
	trained        bool

	// 特征名称
	featureNames []string
}

// IsolationTree 单棵隔离树
type IsolationTree struct {
	root       *IsolationNode
	maxDepth   int
	sampleSize int
}

// IsolationNode 隔离树节点
type IsolationNode struct {
	// 分割信息
	SplitFeature int     // 分割特征索引
	SplitValue   float64 // 分割值

	// 子节点
	Left  *IsolationNode
	Right *IsolationNode

	// 叶子节点信息
	IsLeaf bool
	Size   int // 叶子节点包含的样本数
}

// NewIsolationForest 创建Isolation Forest
func NewIsolationForest(config *DetectorConfig) *IsolationForest {
	return &IsolationForest{
		config:       config,
		trees:        make([]*IsolationTree, 0),
		bufferSize:   config.IFSampleSize * 2,
		featureNames: []string{"battery", "battery_change", "position_change", "latency", "cpu", "memory"},
	}
}

// Name 返回检测器名称
func (iF *IsolationForest) Name() string {
	return "isolation_forest"
}

// AddSample 添加训练样本
func (iF *IsolationForest) AddSample(features []float64) {
	iF.mu.Lock()
	defer iF.mu.Unlock()

	iF.trainingBuffer = append(iF.trainingBuffer, features)

	// 保持缓冲区大小
	if len(iF.trainingBuffer) > iF.bufferSize {
		iF.trainingBuffer = iF.trainingBuffer[len(iF.trainingBuffer)-iF.bufferSize:]
	}

	// 当收集到足够样本时自动训练
	if len(iF.trainingBuffer) >= iF.config.IFSampleSize && !iF.trained {
		iF.trainInternal()
	}
}

// Train 手动触发训练
func (iF *IsolationForest) Train() {
	iF.mu.Lock()
	defer iF.mu.Unlock()
	iF.trainInternal()
}

// trainInternal 内部训练方法（需要持有锁）
func (iF *IsolationForest) trainInternal() {
	if len(iF.trainingBuffer) < 10 {
		return
	}

	// 计算最大深度
	maxDepth := int(math.Ceil(math.Log2(float64(len(iF.trainingBuffer)))))

	// 构建多棵树
	iF.trees = make([]*IsolationTree, iF.config.IFNumTrees)
	for i := 0; i < iF.config.IFNumTrees; i++ {
		// 随机采样
		sample := iF.randomSample(iF.trainingBuffer, iF.config.IFSampleSize)
		iF.trees[i] = iF.buildTree(sample, maxDepth)
	}

	iF.trained = true
}

// randomSample 随机采样
func (iF *IsolationForest) randomSample(data [][]float64, size int) [][]float64 {
	if len(data) <= size {
		return data
	}

	sample := make([][]float64, size)
	perm := rand.Perm(len(data))
	for i := 0; i < size; i++ {
		sample[i] = data[perm[i]]
	}
	return sample
}

// buildTree 构建单棵隔离树
func (iF *IsolationForest) buildTree(data [][]float64, maxDepth int) *IsolationTree {
	tree := &IsolationTree{
		maxDepth:   maxDepth,
		sampleSize: len(data),
	}

	tree.root = iF.buildNode(data, 0, maxDepth)
	return tree
}

// buildNode 递归构建节点
func (iF *IsolationForest) buildNode(data [][]float64, depth, maxDepth int) *IsolationNode {
	// 终止条件
	if depth >= maxDepth || len(data) <= 1 {
		return &IsolationNode{
			IsLeaf: true,
			Size:   len(data),
		}
	}

	// 随机选择特征
	if len(data[0]) == 0 {
		return &IsolationNode{IsLeaf: true, Size: len(data)}
	}
	featureIdx := rand.Intn(len(data[0]))

	// 找到该特征的最小值和最大值
	minVal, maxVal := data[0][featureIdx], data[0][featureIdx]
	for _, sample := range data {
		if sample[featureIdx] < minVal {
			minVal = sample[featureIdx]
		}
		if sample[featureIdx] > maxVal {
			maxVal = sample[featureIdx]
		}
	}

	// 如果所有值相同，创建叶子节点
	if minVal == maxVal {
		return &IsolationNode{IsLeaf: true, Size: len(data)}
	}

	// 随机选择分割值
	splitValue := minVal + rand.Float64()*(maxVal-minVal)

	// 分割数据
	var leftData, rightData [][]float64
	for _, sample := range data {
		if sample[featureIdx] < splitValue {
			leftData = append(leftData, sample)
		} else {
			rightData = append(rightData, sample)
		}
	}

	// 如果分割不均匀，创建叶子节点
	if len(leftData) == 0 || len(rightData) == 0 {
		return &IsolationNode{IsLeaf: true, Size: len(data)}
	}

	return &IsolationNode{
		SplitFeature: featureIdx,
		SplitValue:   splitValue,
		Left:         iF.buildNode(leftData, depth+1, maxDepth),
		Right:        iF.buildNode(rightData, depth+1, maxDepth),
	}
}

// Score 计算异常分数
func (iF *IsolationForest) Score(features []float64) float64 {
	iF.mu.RLock()
	defer iF.mu.RUnlock()

	if !iF.trained || len(iF.trees) == 0 {
		return 0.5 // 未训练时返回中等分数
	}

	// 计算平均路径长度
	var totalPathLength float64
	for _, tree := range iF.trees {
		pathLength := iF.pathLength(features, tree.root, 0)
		totalPathLength += pathLength
	}
	avgPathLength := totalPathLength / float64(len(iF.trees))

	// 计算期望路径长度
	n := float64(iF.config.IFSampleSize)
	expectedPathLength := iF.expectedPathLength(n)

	// 计算异常分数: s = 2^(-avgPathLength/expectedPathLength)
	score := math.Pow(2, -avgPathLength/expectedPathLength)

	return score
}

// pathLength 计算样本在树中的路径长度
func (iF *IsolationForest) pathLength(features []float64, node *IsolationNode, depth int) float64 {
	if node.IsLeaf {
		// 加上未分割数据的期望路径长度
		return float64(depth) + iF.expectedPathLength(float64(node.Size))
	}

	if node.SplitFeature >= len(features) {
		return float64(depth)
	}

	if features[node.SplitFeature] < node.SplitValue {
		return iF.pathLength(features, node.Left, depth+1)
	}
	return iF.pathLength(features, node.Right, depth+1)
}

// expectedPathLength 计算期望路径长度 c(n)
func (iF *IsolationForest) expectedPathLength(n float64) float64 {
	if n <= 1 {
		return 0
	}
	if n == 2 {
		return 1
	}
	// c(n) = 2*H(n-1) - 2*(n-1)/n
	// H(i) ≈ ln(i) + 0.5772156649 (欧拉常数)
	h := math.Log(n-1) + 0.5772156649
	return 2*h - 2*(n-1)/n
}

// Detect 检测异常
func (iF *IsolationForest) Detect(nodeName string, features []float64) *Anomaly {
	score := iF.Score(features)

	// 同时添加到训练缓冲
	iF.AddSample(features)

	// 检查是否超过阈值
	if score >= iF.config.IFThreshold {
		severity := iF.determineSeverity(score)

		return &Anomaly{
			ID:         fmt.Sprintf("if-%s-%d", nodeName, time.Now().UnixNano()),
			NodeName:   nodeName,
			Type:       AnomalyUnknown, // Isolation Forest是无监督的，不知道具体类型
			Severity:   severity,
			Score:      score,
			Message:    fmt.Sprintf("Isolation Forest检测到异常: score=%.3f (阈值=%.3f)", score, iF.config.IFThreshold),
			DetectedAt: time.Now(),
			DetectedBy: iF.Name(),
			Threshold:  iF.config.IFThreshold,
		}
	}

	return nil
}

// determineSeverity 根据分数确定严重程度
func (iF *IsolationForest) determineSeverity(score float64) AnomalySeverity {
	switch {
	case score >= 0.9:
		return SeverityFatal
	case score >= 0.8:
		return SeverityCritical
	case score >= 0.7:
		return SeverityWarning
	default:
		return SeverityInfo
	}
}

// IsTrained 检查是否已训练
func (iF *IsolationForest) IsTrained() bool {
	iF.mu.RLock()
	defer iF.mu.RUnlock()
	return iF.trained
}

// GetStats 获取统计信息
func (iF *IsolationForest) GetStats() map[string]interface{} {
	iF.mu.RLock()
	defer iF.mu.RUnlock()

	return map[string]interface{}{
		"trained":        iF.trained,
		"num_trees":      len(iF.trees),
		"buffer_size":    len(iF.trainingBuffer),
		"feature_names":  iF.featureNames,
	}
}

// Reset 重置检测器
func (iF *IsolationForest) Reset() {
	iF.mu.Lock()
	defer iF.mu.Unlock()

	iF.trees = make([]*IsolationTree, 0)
	iF.trainingBuffer = nil
	iF.trained = false
}

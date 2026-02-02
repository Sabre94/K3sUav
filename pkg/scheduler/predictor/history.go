package predictor

import (
	"sync"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// HistoryBuffer 单个节点的历史数据环形缓冲区
type HistoryBuffer struct {
	NodeName string
	Points   []HistoryPoint
	MaxSize  int
	head     int // 下一个写入位置
	count    int // 当前数据点数
	mu       sync.RWMutex
}

// NewHistoryBuffer 创建历史缓冲区
func NewHistoryBuffer(nodeName string, maxSize int) *HistoryBuffer {
	return &HistoryBuffer{
		NodeName: nodeName,
		Points:   make([]HistoryPoint, maxSize),
		MaxSize:  maxSize,
		head:     0,
		count:    0,
	}
}

// Add 添加新数据点
func (hb *HistoryBuffer) Add(point HistoryPoint) {
	hb.mu.Lock()
	defer hb.mu.Unlock()

	hb.Points[hb.head] = point
	hb.head = (hb.head + 1) % hb.MaxSize
	if hb.count < hb.MaxSize {
		hb.count++
	}
}

// AddFromMetrics 从UAVMetrics添加数据点
func (hb *HistoryBuffer) AddFromMetrics(metrics *models.UAVMetrics, timestamp time.Time) {
	point := HistoryPoint{
		Timestamp: timestamp,
		Battery:   metrics.Battery.RemainingPercent,
	}

	// 位置
	if metrics.Position != nil {
		point.Position = Position{
			X: metrics.Position.X,
			Y: metrics.Position.Y,
			Z: metrics.Position.Z,
		}
	}

	// 速度
	if metrics.Velocity != nil {
		point.Velocity = Velocity{
			Vx: metrics.Velocity.Vx,
			Vy: metrics.Velocity.Vy,
			Vz: metrics.Velocity.Vz,
		}
	}

	// 飞行速度
	if metrics.GPS.Speed > 0 {
		point.Speed = metrics.GPS.Speed
	}

	// 网络延迟
	if metrics.Network != nil {
		point.Latency = metrics.Network.Latency
	}

	hb.Add(point)
}

// GetLatest 获取最新的数据点
func (hb *HistoryBuffer) GetLatest() (HistoryPoint, bool) {
	hb.mu.RLock()
	defer hb.mu.RUnlock()

	if hb.count == 0 {
		return HistoryPoint{}, false
	}

	idx := (hb.head - 1 + hb.MaxSize) % hb.MaxSize
	return hb.Points[idx], true
}

// GetLastN 获取最近N个数据点（按时间从旧到新排序）
func (hb *HistoryBuffer) GetLastN(n int) []HistoryPoint {
	hb.mu.RLock()
	defer hb.mu.RUnlock()

	if n > hb.count {
		n = hb.count
	}
	if n == 0 {
		return nil
	}

	result := make([]HistoryPoint, n)
	for i := 0; i < n; i++ {
		// 从最旧的开始
		idx := (hb.head - n + i + hb.MaxSize) % hb.MaxSize
		result[i] = hb.Points[idx]
	}
	return result
}

// GetAll 获取所有数据点（按时间从旧到新排序）
func (hb *HistoryBuffer) GetAll() []HistoryPoint {
	return hb.GetLastN(hb.count)
}

// Count 返回当前数据点数量
func (hb *HistoryBuffer) Count() int {
	hb.mu.RLock()
	defer hb.mu.RUnlock()
	return hb.count
}

// Clear 清空缓冲区
func (hb *HistoryBuffer) Clear() {
	hb.mu.Lock()
	defer hb.mu.Unlock()
	hb.head = 0
	hb.count = 0
}

// HistoryManager 管理所有节点的历史数据
type HistoryManager struct {
	buffers map[string]*HistoryBuffer
	config  *PredictorConfig
	mu      sync.RWMutex
}

// NewHistoryManager 创建历史管理器
func NewHistoryManager(config *PredictorConfig) *HistoryManager {
	return &HistoryManager{
		buffers: make(map[string]*HistoryBuffer),
		config:  config,
	}
}

// GetOrCreateBuffer 获取或创建节点的历史缓冲区
func (hm *HistoryManager) GetOrCreateBuffer(nodeName string) *HistoryBuffer {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if buffer, exists := hm.buffers[nodeName]; exists {
		return buffer
	}

	buffer := NewHistoryBuffer(nodeName, hm.config.HistorySize)
	hm.buffers[nodeName] = buffer
	return buffer
}

// GetBuffer 获取节点的历史缓冲区
func (hm *HistoryManager) GetBuffer(nodeName string) (*HistoryBuffer, bool) {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	buffer, exists := hm.buffers[nodeName]
	return buffer, exists
}

// UpdateFromMetrics 从UAVMetrics更新历史数据
func (hm *HistoryManager) UpdateFromMetrics(metrics *models.UAVMetrics) {
	buffer := hm.GetOrCreateBuffer(metrics.NodeName)

	// 使用GPS的LastUpdate作为时间戳，如果没有则用当前时间
	timestamp := metrics.GPS.LastUpdate
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	buffer.AddFromMetrics(metrics, timestamp)
}

// UpdateAllFromMetrics 批量更新
func (hm *HistoryManager) UpdateAllFromMetrics(metricsList []*models.UAVMetrics) {
	for _, metrics := range metricsList {
		hm.UpdateFromMetrics(metrics)
	}
}

// GetNodeCount 获取节点数量
func (hm *HistoryManager) GetNodeCount() int {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return len(hm.buffers)
}

// GetAllNodeNames 获取所有节点名称
func (hm *HistoryManager) GetAllNodeNames() []string {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	names := make([]string, 0, len(hm.buffers))
	for name := range hm.buffers {
		names = append(names, name)
	}
	return names
}

// RemoveNode 移除节点历史数据
func (hm *HistoryManager) RemoveNode(nodeName string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	delete(hm.buffers, nodeName)
}

// Clear 清空所有历史数据
func (hm *HistoryManager) Clear() {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.buffers = make(map[string]*HistoryBuffer)
}

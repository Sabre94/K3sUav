package algorithm

import (
	"context"
	"fmt"

	"github.com/k3suav/uav-monitor/pkg/models"
	v1 "k8s.io/api/core/v1"
)

// LeastLoadedAlgorithm K8s 默认调度器风格的算法
// 优先选择资源利用率最低的节点（CPU + 内存）
// 模拟 K8s Default Scheduler 的 LeastAllocated 策略
type LeastLoadedAlgorithm struct {
	CPUWeight    float64 // CPU 权重
	MemoryWeight float64 // 内存权重
}

// NewLeastLoadedAlgorithm 创建 Least-Loaded 算法
func NewLeastLoadedAlgorithm() *LeastLoadedAlgorithm {
	return &LeastLoadedAlgorithm{
		CPUWeight:    0.5, // CPU 和内存各占 50%
		MemoryWeight: 0.5,
	}
}

func (a *LeastLoadedAlgorithm) Name() string {
	return "least-loaded"
}

func (a *LeastLoadedAlgorithm) Filter(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]*models.UAVMetrics, error) {
	// 不做硬性过滤，让所有节点参与评分
	return metrics, nil
}

func (a *LeastLoadedAlgorithm) Score(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]NodeScore, error) {
	scores := []NodeScore{}

	for _, m := range metrics {
		// 计算资源可用性
		// K8s LeastAllocated: score = (capacity - allocated) / capacity * 100
		// 这里我们用 (100 - usage) 来近似可用资源

		cpuAvailable := 100.0 - m.Performance.CPUUsage
		memoryAvailable := 100.0 - m.Performance.MemoryUsage

		// 加权平均
		score := a.CPUWeight*cpuAvailable + a.MemoryWeight*memoryAvailable

		// 确保分数在 [0, 100] 范围内
		if score < 0 {
			score = 0
		}
		if score > 100 {
			score = 100
		}

		scores = append(scores, NodeScore{
			NodeName: m.NodeName,
			Score:    score,
			Reason: fmt.Sprintf("cpu_avail: %.1f%%, mem_avail: %.1f%%, combined: %.1f",
				cpuAvailable, memoryAvailable, score),
		})
	}

	return scores, nil
}

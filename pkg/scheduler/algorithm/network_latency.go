package algorithm

import (
	"context"
	"fmt"

	"github.com/k3suav/uav-monitor/pkg/models"
	v1 "k8s.io/api/core/v1"
)

// NetworkLatencyAlgorithm 基于网络延迟的调度算法
// 优先选择网络延迟低的节点
type NetworkLatencyAlgorithm struct {
	MaxLatency float64 // 最大可接受延迟（毫秒）
}

// NewNetworkLatencyAlgorithm 创建基于网络延迟的算法
func NewNetworkLatencyAlgorithm(maxLatency float64) *NetworkLatencyAlgorithm {
	return &NetworkLatencyAlgorithm{
		MaxLatency: maxLatency,
	}
}

func (a *NetworkLatencyAlgorithm) Name() string {
	return "network-latency"
}

func (a *NetworkLatencyAlgorithm) Filter(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]*models.UAVMetrics, error) {
	filtered := []*models.UAVMetrics{}

	// 过滤掉延迟过高的节点
	for _, m := range metrics {
		if m.Network != nil && m.Network.Latency <= a.MaxLatency {
			filtered = append(filtered, m)
		}
	}

	return filtered, nil
}

func (a *NetworkLatencyAlgorithm) Score(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]NodeScore, error) {
	scores := []NodeScore{}

	for _, m := range metrics {
		if m.Network == nil {
			// 没有网络数据，给最低分
			scores = append(scores, NodeScore{
				NodeName: m.NodeName,
				Score:    0,
				Reason:   "no network data",
			})
			continue
		}

		latency := m.Network.Latency

		// 延迟越低，分数越高
		// score = 100 * (1 - latency/maxLatency)
		score := 100.0 * (1.0 - latency/a.MaxLatency)
		if score < 0 {
			score = 0
		}

		scores = append(scores, NodeScore{
			NodeName: m.NodeName,
			Score:    score,
			Reason:   fmt.Sprintf("latency: %.1fms (max: %.1fms)", latency, a.MaxLatency),
		})
	}

	return scores, nil
}

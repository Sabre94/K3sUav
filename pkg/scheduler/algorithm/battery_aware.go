package algorithm

import (
	"context"
	"fmt"

	"github.com/k3suav/uav-monitor/pkg/models"
	v1 "k8s.io/api/core/v1"
)

// BatteryAwareAlgorithm 基于电池的调度算法
// 优先选择电池电量充足的节点
type BatteryAwareAlgorithm struct {
	MinBattery float64 // 最低电池电量要求（百分比）
}

// NewBatteryAwareAlgorithm 创建基于电池的算法
func NewBatteryAwareAlgorithm(minBattery float64) *BatteryAwareAlgorithm {
	return &BatteryAwareAlgorithm{
		MinBattery: minBattery,
	}
}

func (a *BatteryAwareAlgorithm) Name() string {
	return "battery-aware"
}

func (a *BatteryAwareAlgorithm) Filter(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]*models.UAVMetrics, error) {
	filtered := []*models.UAVMetrics{}

	// 过滤掉电量不足的节点
	for _, m := range metrics {
		if m.Battery.RemainingPercent >= a.MinBattery {
			filtered = append(filtered, m)
		}
	}

	return filtered, nil
}

func (a *BatteryAwareAlgorithm) Score(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]NodeScore, error) {
	scores := []NodeScore{}

	for _, m := range metrics {
		// 电池电量直接作为分数（0-100）
		score := m.Battery.RemainingPercent

		// 如果电量低于最低要求，分数为0
		if score < a.MinBattery {
			score = 0
		}

		scores = append(scores, NodeScore{
			NodeName: m.NodeName,
			Score:    score,
			Reason:   fmt.Sprintf("battery: %.1f%% (min: %.1f%%)", m.Battery.RemainingPercent, a.MinBattery),
		})
	}

	return scores, nil
}

package algorithm

import (
	"context"
	"fmt"

	"github.com/k3suav/uav-monitor/pkg/models"
	v1 "k8s.io/api/core/v1"
)

// DistanceBasedAlgorithm 基于距离的调度算法
// 选择距离目标位置最近的节点
type DistanceBasedAlgorithm struct {
	TargetLocation Location // 目标位置
}

// NewDistanceBasedAlgorithm 创建基于距离的算法
func NewDistanceBasedAlgorithm(targetLat, targetLon float64) *DistanceBasedAlgorithm {
	return &DistanceBasedAlgorithm{
		TargetLocation: Location{
			Latitude:  targetLat,
			Longitude: targetLon,
		},
	}
}

func (a *DistanceBasedAlgorithm) Name() string {
	return "distance-based"
}

func (a *DistanceBasedAlgorithm) Filter(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]*models.UAVMetrics, error) {
	// 不做硬性过滤
	return metrics, nil
}

func (a *DistanceBasedAlgorithm) Score(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]NodeScore, error) {
	scores := []NodeScore{}

	// 如果从 Pod 注解中获取目标位置，优先使用
	if lat, ok := pod.Annotations["uav.scheduler/target-lat"]; ok {
		if lon, ok := pod.Annotations["uav.scheduler/target-lon"]; ok {
			// 解析目标位置
			var targetLat, targetLon float64
			fmt.Sscanf(lat, "%f", &targetLat)
			fmt.Sscanf(lon, "%f", &targetLon)
			a.TargetLocation.Latitude = targetLat
			a.TargetLocation.Longitude = targetLon
		}
	}

	for _, m := range metrics {
		// 计算节点与目标位置的距离
		distance := CalculateDistance(
			m.GPS.Latitude, m.GPS.Longitude,
			a.TargetLocation.Latitude, a.TargetLocation.Longitude,
		)

		// 距离越近，分数越高
		// 使用反比关系：score = 100 / (1 + distance)
		// distance=0时score=100，distance增加时score递减
		score := 100.0 / (1.0 + distance)

		scores = append(scores, NodeScore{
			NodeName: m.NodeName,
			Score:    score,
			Reason:   fmt.Sprintf("distance: %.2fkm from target (%.4f,%.4f)", distance, a.TargetLocation.Latitude, a.TargetLocation.Longitude),
		})
	}

	return scores, nil
}

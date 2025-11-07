package algorithm

import (
	"context"
	"fmt"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// BatteryAwareRouter 基于电量的路由算法
// 优先将流量路由到电量充足的 Pod，避免向低电量节点发送请求
type BatteryAwareRouter struct {
	// MinBattery 最低电量阈值（百分比），低于此值的节点将被过滤
	MinBattery float64
}

// NewBatteryAwareRouter 创建基于电量的路由算法实例
func NewBatteryAwareRouter(minBattery float64) *BatteryAwareRouter {
	if minBattery <= 0 {
		minBattery = 20.0 // 默认最低 20% 电量
	}
	return &BatteryAwareRouter{
		MinBattery: minBattery,
	}
}

// Name 返回算法名称
func (r *BatteryAwareRouter) Name() string {
	return "battery-aware"
}

// ComputeWeights 计算基于电量的路由权重
func (r *BatteryAwareRouter) ComputeWeights(
	ctx context.Context,
	sourceNode string,
	sourceMetrics *models.UAVMetrics,
	targetEndpoints []Endpoint,
	targetMetrics map[string]*models.UAVMetrics,
) ([]EndpointWeight, error) {

	weights := make([]EndpointWeight, 0, len(targetEndpoints))

	for _, ep := range targetEndpoints {
		// 获取目标节点的指标
		targetM, exists := targetMetrics[ep.NodeName]
		if !exists {
			continue
		}

		// 电量过滤：低于最低电量的节点不参与路由
		if targetM.Battery.RemainingPercent < r.MinBattery {
			continue
		}

		// 计算权重：电量越高权重越高
		// 直接使用电量百分比作为权重基础
		weight := targetM.Battery.RemainingPercent

		// 添加非线性加权：高电量节点获得额外奖励
		if targetM.Battery.RemainingPercent > 80 {
			weight *= 1.2 // 80% 以上电量，权重提升 20%
		} else if targetM.Battery.RemainingPercent < 30 {
			weight *= 0.8 // 30% 以下电量，权重降低 20%
		}

		// 确保权重在 1-100 范围内
		if weight > 100 {
			weight = 100
		}
		if weight < 1 {
			weight = 1
		}

		weights = append(weights, EndpointWeight{
			Endpoint: ep,
			Weight:   int(weight),
			Priority: 0,
			Reason: fmt.Sprintf("battery: %.1f%%, voltage: %.2fV",
				targetM.Battery.RemainingPercent, targetM.Battery.Voltage),
		})
	}

	if len(weights) == 0 {
		return nil, fmt.Errorf("no eligible endpoints found with battery >= %.1f%%", r.MinBattery)
	}

	return weights, nil
}

package algorithm

import (
	"context"

	"github.com/k3suav/uav-monitor/pkg/models"
	v1 "k8s.io/api/core/v1"
)

// SchedulingAlgorithm 调度算法接口 - 所有自定义算法必须实现此接口
type SchedulingAlgorithm interface {
	// Name 返回算法名称
	Name() string

	// Score 为每个节点计算分数
	// 输入：Pod 信息 + 所有节点的 UAVMetrics
	// 输出：每个节点的分数（0-100，越高越好）
	Score(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]NodeScore, error)

	// Filter 过滤不符合条件的节点（可选，返回 nil 表示不过滤）
	// 如果需要硬性过滤某些节点，实现此方法
	Filter(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]*models.UAVMetrics, error)
}

// NodeScore 节点评分结果
type NodeScore struct {
	NodeName string  // 节点名称
	Score    float64 // 分数 (0-100)
	Reason   string  // 评分原因（用于日志和调试）
}

// Location GPS 位置
type Location struct {
	Latitude  float64
	Longitude float64
}

// CalculateDistance 计算两个 GPS 坐标之间的距离（单位：公里）
// 使用 Haversine 公式
func CalculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0 // 地球半径（公里）

	// 转换为弧度
	lat1Rad := lat1 * 3.14159265359 / 180
	lat2Rad := lat2 * 3.14159265359 / 180
	deltaLat := (lat2 - lat1) * 3.14159265359 / 180
	deltaLon := (lon2 - lon1) * 3.14159265359 / 180

	// Haversine 公式
	a := 0.5 - 0.5*cos(deltaLat) + cos(lat1Rad)*cos(lat2Rad)*(0.5-0.5*cos(deltaLon))

	return 2 * R * asin(sqrt(a))
}

// 辅助数学函数
func cos(x float64) float64 {
	// 泰勒级数近似（精度足够）
	x2 := x * x
	return 1 - x2/2 + x2*x2/24 - x2*x2*x2/720
}

func sin(x float64) float64 {
	x2 := x * x
	return x - x*x2/6 + x*x2*x2/120
}

func asin(x float64) float64 {
	if x > 1.0 {
		x = 1.0
	}
	if x < -1.0 {
		x = -1.0
	}
	return atan2(x, sqrt(1-x*x))
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

func atan2(y, x float64) float64 {
	if x > 0 {
		return atan(y / x)
	}
	if x < 0 && y >= 0 {
		return atan(y/x) + 3.14159265359
	}
	if x < 0 && y < 0 {
		return atan(y/x) - 3.14159265359
	}
	if y > 0 {
		return 3.14159265359 / 2
	}
	return -3.14159265359 / 2
}

func atan(x float64) float64 {
	// 泰勒级数近似
	if x > 1 {
		return 3.14159265359/2 - atan(1/x)
	}
	if x < -1 {
		return -3.14159265359/2 - atan(1/x)
	}
	x2 := x * x
	return x - x*x2/3 + x*x2*x2/5 - x*x2*x2*x2/7
}

package algorithm

import (
	"fmt"
	"strconv"

	v1 "k8s.io/api/core/v1"
)

// AlgorithmFactory 算法工厂
// 根据 Pod 的 annotation 动态创建算法实例
type AlgorithmFactory struct{}

// NewAlgorithmFactory 创建算法工厂
func NewAlgorithmFactory() *AlgorithmFactory {
	return &AlgorithmFactory{}
}

// CreateFromPod 从 Pod annotation 创建算法实例
// 支持的 annotations:
//   - uav.scheduler/algorithm: 算法名称 (distance-based, battery-aware, network-latency, composite)
//   - uav.scheduler/target-lat: 目标纬度 (distance-based)
//   - uav.scheduler/target-lon: 目标经度 (distance-based)
//   - uav.scheduler/min-battery: 最低电池百分比 (battery-aware)
//   - uav.scheduler/max-latency: 最大延迟毫秒 (network-latency)
//   - uav.scheduler/composite-weights: 组合算法权重，格式: "0.6,0.4" (composite)
func (f *AlgorithmFactory) CreateFromPod(pod *v1.Pod, defaultAlgo SchedulingAlgorithm) (SchedulingAlgorithm, error) {
	if pod.Annotations == nil {
		return defaultAlgo, nil
	}

	// 读取算法名称
	algoName := pod.Annotations["uav.scheduler/algorithm"]
	if algoName == "" {
		// 如果 Pod 没有指定算法，使用默认算法
		return defaultAlgo, nil
	}

	// 根据算法名称创建实例
	switch algoName {
	case "distance-based":
		return f.createDistanceBased(pod)

	case "battery-aware":
		return f.createBatteryAware(pod)

	case "network-latency":
		return f.createNetworkLatency(pod)

	case "composite":
		return f.createComposite(pod)

	default:
		return nil, fmt.Errorf("unsupported algorithm '%s' in pod annotation", algoName)
	}
}

// createDistanceBased 创建基于距离的算法
func (f *AlgorithmFactory) createDistanceBased(pod *v1.Pod) (SchedulingAlgorithm, error) {
	// 读取目标坐标
	targetLat := getFloatAnnotation(pod, "uav.scheduler/target-lat", 34.0522)
	targetLon := getFloatAnnotation(pod, "uav.scheduler/target-lon", -118.2437)

	return NewDistanceBasedAlgorithm(targetLat, targetLon), nil
}

// createBatteryAware 创建基于电池的算法
func (f *AlgorithmFactory) createBatteryAware(pod *v1.Pod) (SchedulingAlgorithm, error) {
	// 读取最低电池要求
	minBattery := getFloatAnnotation(pod, "uav.scheduler/min-battery", 30.0)

	return NewBatteryAwareAlgorithm(minBattery), nil
}

// createNetworkLatency 创建基于网络延迟的算法
func (f *AlgorithmFactory) createNetworkLatency(pod *v1.Pod) (SchedulingAlgorithm, error) {
	// 读取最大延迟
	maxLatency := getFloatAnnotation(pod, "uav.scheduler/max-latency", 200.0)

	return NewNetworkLatencyAlgorithm(maxLatency), nil
}

// createComposite 创建组合算法
func (f *AlgorithmFactory) createComposite(pod *v1.Pod) (SchedulingAlgorithm, error) {
	// 读取目标坐标（用于 distance-based 子算法）
	targetLat := getFloatAnnotation(pod, "uav.scheduler/target-lat", 34.0522)
	targetLon := getFloatAnnotation(pod, "uav.scheduler/target-lon", -118.2437)

	// 读取最低电池（用于 battery-aware 子算法）
	minBattery := getFloatAnnotation(pod, "uav.scheduler/min-battery", 30.0)

	// 创建子算法
	distanceAlgo := NewDistanceBasedAlgorithm(targetLat, targetLon)
	batteryAlgo := NewBatteryAwareAlgorithm(minBattery)

	// 读取权重，默认 60% 距离 + 40% 电池
	weightsStr := pod.Annotations["uav.scheduler/composite-weights"]
	weights := []float64{0.6, 0.4}

	if weightsStr != "" {
		// 解析权重字符串，格式: "0.7,0.3"
		parsedWeights, err := parseWeights(weightsStr)
		if err == nil && len(parsedWeights) == 2 {
			weights = parsedWeights
		}
	}

	return NewCompositeAlgorithm(
		[]SchedulingAlgorithm{distanceAlgo, batteryAlgo},
		weights,
	), nil
}

// 辅助函数：从 annotation 读取 float64 值
func getFloatAnnotation(pod *v1.Pod, key string, defaultValue float64) float64 {
	if pod.Annotations == nil {
		return defaultValue
	}

	value := pod.Annotations[key]
	if value == "" {
		return defaultValue
	}

	floatValue, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return defaultValue
	}

	return floatValue
}

// 辅助函数：解析权重字符串
func parseWeights(weightsStr string) ([]float64, error) {
	weights := []float64{}

	// 简单的逗号分隔解析
	var w1, w2 float64
	_, err := fmt.Sscanf(weightsStr, "%f,%f", &w1, &w2)
	if err != nil {
		return nil, err
	}

	weights = append(weights, w1, w2)
	return weights, nil
}

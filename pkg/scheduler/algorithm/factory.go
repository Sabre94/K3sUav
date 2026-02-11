package algorithm

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/greed_nsgaii"
	v1 "k8s.io/api/core/v1"
)

// AlgorithmCreatorFunc 外部算法创建函数
type AlgorithmCreatorFunc func(pod *v1.Pod) (SchedulingAlgorithm, error)

// AlgorithmFactory 算法工厂
// 根据 Pod 的 annotation 动态创建算法实例
type AlgorithmFactory struct {
	// coverage-based 算法的单例缓存（key: "coverage-<requirement>-<radius>"）
	coverageAlgos map[string]*CoverageBasedAlgorithm
	// greed-nsgaii 算法的单例缓存（key: "greed-<tasktype>-<coverage>-<radius>"）
	greedNSGAIIAlgos map[string]*greed_nsgaii.GreedNSGAIIAlgorithm
	// 外部注册的算法创建函数（避免循环导入）
	externalCreators map[string]AlgorithmCreatorFunc
	mu               sync.RWMutex
}

// NewAlgorithmFactory 创建算法工厂
func NewAlgorithmFactory() *AlgorithmFactory {
	return &AlgorithmFactory{
		coverageAlgos:    make(map[string]*CoverageBasedAlgorithm),
		greedNSGAIIAlgos: make(map[string]*greed_nsgaii.GreedNSGAIIAlgorithm),
		externalCreators: make(map[string]AlgorithmCreatorFunc),
	}
}

// RegisterCreator 注册外部算法创建函数
func (f *AlgorithmFactory) RegisterCreator(name string, creator AlgorithmCreatorFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.externalCreators[name] = creator
}

// CreateFromPod 从 Pod annotation 创建算法实例
// 支持的 annotations:
//   - uav.scheduler/algorithm: 算法名称 (distance-based, battery-aware, network-latency, composite, coverage-based, greed-nsgaii, rl-coverage)
//   - uav.scheduler/target-lat: 目标纬度 (distance-based)
//   - uav.scheduler/target-lon: 目标经度 (distance-based)
//   - uav.scheduler/min-battery: 最低电池百分比 (battery-aware)
//   - uav.scheduler/max-latency: 最大延迟毫秒 (network-latency)
//   - uav.scheduler/composite-weights: 组合算法权重，格式: "0.6,0.4" (composite)
//   - uav.scheduler/task-type: 任务类型 (greed-nsgaii): emergency, sustain, compute, default
//   - uav.scheduler/target-coverage: 目标覆盖率 (greed-nsgaii, rl-coverage): 0.0-1.0
//   - uav.scheduler/coverage-radius: 覆盖半径（米）(greed-nsgaii, rl-coverage)
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

	case "coverage-based":
		return f.createCoverageBased(pod)

	case "greed-nsgaii":
		return f.createGreedNSGAII(pod)

	default:
		// 检查外部注册的算法创建函数
		f.mu.RLock()
		creator, exists := f.externalCreators[algoName]
		f.mu.RUnlock()
		if exists {
			return creator(pod)
		}
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

// createCoverageBased 创建基于覆盖率的算法（单例模式）
func (f *AlgorithmFactory) createCoverageBased(pod *v1.Pod) (SchedulingAlgorithm, error) {
	// 读取覆盖率要求
	coverageRequirement := getFloatAnnotation(pod, "uav.scheduler/coverage-requirement", 90.0)

	// 读取覆盖半径
	coverageRadius := getFloatAnnotation(pod, "uav.scheduler/coverage-radius", 5.0)

	// 生成缓存 key（相同配置的 Deployment 共享同一个算法实例）
	key := fmt.Sprintf("coverage-%.1f-%.1f", coverageRequirement, coverageRadius)

	// 检查是否已存在算法实例（读锁）
	f.mu.RLock()
	algo, exists := f.coverageAlgos[key]
	f.mu.RUnlock()

	if exists {
		return algo, nil // 复用已有实例
	}

	// 创建新实例（写锁）
	f.mu.Lock()
	defer f.mu.Unlock()

	// Double-check（避免并发创建）
	if algo, exists := f.coverageAlgos[key]; exists {
		return algo, nil
	}

	// 创建并缓存
	algo = NewCoverageBasedAlgorithm(coverageRequirement, coverageRadius)
	f.coverageAlgos[key] = algo
	return algo, nil
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

// createGreedNSGAII 创建 GREED + NSGA-II 算法（单例模式）
func (f *AlgorithmFactory) createGreedNSGAII(pod *v1.Pod) (SchedulingAlgorithm, error) {
	// 读取任务类型
	taskTypeStr := pod.Annotations["uav.scheduler/task-type"]
	if taskTypeStr == "" {
		taskTypeStr = "default"
	}

	var taskType greed_nsgaii.TaskType
	switch taskTypeStr {
	case "emergency":
		taskType = greed_nsgaii.TaskTypeEmergency
	case "sustain":
		taskType = greed_nsgaii.TaskTypeSustain
	case "compute":
		taskType = greed_nsgaii.TaskTypeCompute
	default:
		taskType = greed_nsgaii.TaskTypeDefault
	}

	// 读取目标覆盖率（0.0 - 1.0）
	targetCoverage := getFloatAnnotation(pod, "uav.scheduler/target-coverage", 0.9)
	if targetCoverage < 0 {
		targetCoverage = 0
	}
	if targetCoverage > 1.0 {
		targetCoverage = 1.0
	}

	// 读取覆盖半径（米）
	coverageRadius := getFloatAnnotation(pod, "uav.scheduler/coverage-radius", 200.0)

	// 生成缓存 key（相同配置的 Deployment 共享同一个算法实例）
	key := fmt.Sprintf("greed-%s-%.2f-%.1f", taskTypeStr, targetCoverage, coverageRadius)

	// 检查是否已存在算法实例（读锁）
	f.mu.RLock()
	algo, exists := f.greedNSGAIIAlgos[key]
	f.mu.RUnlock()

	if exists {
		return &GreedNSGAIIAdapter{algo: algo}, nil // 复用已有实例
	}

	// 创建新实例（写锁）
	f.mu.Lock()
	defer f.mu.Unlock()

	// Double-check（避免并发创建）
	if algo, exists := f.greedNSGAIIAlgos[key]; exists {
		return &GreedNSGAIIAdapter{algo: algo}, nil
	}

	// 创建并缓存
	algo = greed_nsgaii.NewGreedNSGAIIAlgorithm(taskType, targetCoverage, coverageRadius)
	f.greedNSGAIIAlgos[key] = algo

	// 返回适配器包装
	return &GreedNSGAIIAdapter{algo: algo}, nil
}

// GreedNSGAIIAdapter 适配器，用于将 greed_nsgaii.GreedNSGAIIAlgorithm 适配到 SchedulingAlgorithm 接口
type GreedNSGAIIAdapter struct {
	algo *greed_nsgaii.GreedNSGAIIAlgorithm
}

// NewGreedNSGAIIAdapter 创建 GREED-NSGAII 算法适配器
func NewGreedNSGAIIAdapter(taskType greed_nsgaii.TaskType, targetCoverage, coverageRadius float64) *GreedNSGAIIAdapter {
	algo := greed_nsgaii.NewGreedNSGAIIAlgorithm(taskType, targetCoverage, coverageRadius)
	return &GreedNSGAIIAdapter{algo: algo}
}

func (a *GreedNSGAIIAdapter) Name() string {
	return a.algo.Name()
}

func (a *GreedNSGAIIAdapter) Filter(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]*models.UAVMetrics, error) {
	return a.algo.Filter(ctx, pod, metrics)
}

func (a *GreedNSGAIIAdapter) Score(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]NodeScore, error) {
	// 调用底层算法的 Score 方法
	scores, err := a.algo.Score(ctx, pod, metrics)
	if err != nil {
		return nil, err
	}

	// 转换类型
	result := make([]NodeScore, len(scores))
	for i, s := range scores {
		result[i] = NodeScore{
			NodeName: s.NodeName,
			Score:    s.Score,
			Reason:   s.Reason,
		}
	}

	return result, nil
}

// GetUnderlyingAlgorithm 返回底层的 GreedNSGAIIAlgorithm（用于 RecordBinding 等操作）
func (a *GreedNSGAIIAdapter) GetUnderlyingAlgorithm() *greed_nsgaii.GreedNSGAIIAlgorithm {
	return a.algo
}


package config

import (
	"fmt"
	"os"
	"time"
)

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	// 调度器名称（Pod 的 schedulerName 字段必须匹配此值）
	SchedulerName string

	// 使用的算法名称
	AlgorithmName string

	// 算法参数
	AlgorithmParams AlgorithmParams

	// Kubernetes 配置
	KubeconfigPath string
	Namespace      string

	// 调度器行为
	WorkerThreads int           // 并发调度线程数
	RetryAttempts int           // 失败重试次数
	RetryDelay    time.Duration // 重试延迟

	// 日志配置
	LogLevel          string
	StructuredLogging bool

	// 自适应调度配置
	EnableAdaptiveScheduling bool            // 是否启用自适应调度
	AdaptiveParams           AdaptiveParams  // 自适应调度参数
}

// AdaptiveParams 自适应调度参数
type AdaptiveParams struct {
	// 覆盖率阈值
	TargetCoverage     float64 // 目标覆盖率 (0.9 = 90%)
	MinCoverage        float64 // 最低可接受覆盖率 (0.7 = 70%)

	// 变化阈值
	MinorDropThreshold float64 // 小幅下降阈值 (0.1 = 10%)
	MajorDropThreshold float64 // 大幅下降阈值 (0.3 = 30%)

	// 算法配置
	CoverageRadius     float64 // 覆盖半径（米）
	TaskType           string  // 任务类型: default, emergency, sustain, compute

	// 执行配置
	AutoExecute        bool    // 是否自动执行调度动作
}

// AlgorithmParams 算法参数
type AlgorithmParams struct {
	// Distance-based 算法参数
	TargetLatitude  float64
	TargetLongitude float64

	// Battery-aware 算法参数
	MinBattery float64

	// Network-latency 算法参数
	MaxLatency float64

	// Composite 算法参数
	CompositeAlgorithms []string  // 子算法名称列表
	CompositeWeights    []float64 // 对应权重
}

// DefaultConfig 返回默认配置
func DefaultConfig() *SchedulerConfig {
	return &SchedulerConfig{
		SchedulerName:   getEnvOrDefault("SCHEDULER_NAME", "uav-scheduler"),
		AlgorithmName:   getEnvOrDefault("ALGORITHM_NAME", "distance-based"),
		KubeconfigPath:  getEnvOrDefault("KUBECONFIG", ""),
		Namespace:       getEnvOrDefault("NAMESPACE", "default"),
		WorkerThreads:   getEnvIntOrDefault("WORKER_THREADS", 1),
		RetryAttempts:   3,
		RetryDelay:      2 * time.Second,
		LogLevel:        getEnvOrDefault("LOG_LEVEL", "info"),
		StructuredLogging: getEnvBoolOrDefault("STRUCTURED_LOGGING", false),
		AlgorithmParams: AlgorithmParams{
			TargetLatitude:  getEnvFloatOrDefault("TARGET_LATITUDE", 34.0522),
			TargetLongitude: getEnvFloatOrDefault("TARGET_LONGITUDE", -118.2437),
			MinBattery:      getEnvFloatOrDefault("MIN_BATTERY", 30.0),
			MaxLatency:      getEnvFloatOrDefault("MAX_LATENCY", 200.0),
		},
		// 自适应调度配置
		EnableAdaptiveScheduling: getEnvBoolOrDefault("ENABLE_ADAPTIVE_SCHEDULING", false),
		AdaptiveParams: AdaptiveParams{
			TargetCoverage:     getEnvFloatOrDefault("ADAPTIVE_TARGET_COVERAGE", 0.90),
			MinCoverage:        getEnvFloatOrDefault("ADAPTIVE_MIN_COVERAGE", 0.70),
			MinorDropThreshold: getEnvFloatOrDefault("ADAPTIVE_MINOR_DROP", 0.10),
			MajorDropThreshold: getEnvFloatOrDefault("ADAPTIVE_MAJOR_DROP", 0.30),
			CoverageRadius:     getEnvFloatOrDefault("ADAPTIVE_COVERAGE_RADIUS", 500.0),
			TaskType:           getEnvOrDefault("ADAPTIVE_TASK_TYPE", "default"),
			AutoExecute:        getEnvBoolOrDefault("ADAPTIVE_AUTO_EXECUTE", true),
		},
	}
}

// Validate 验证配置
func (c *SchedulerConfig) Validate() error {
	if c.SchedulerName == "" {
		return fmt.Errorf("schedulerName cannot be empty")
	}
	if c.AlgorithmName == "" {
		return fmt.Errorf("algorithmName cannot be empty")
	}
	if c.WorkerThreads < 1 {
		return fmt.Errorf("workerThreads must be >= 1")
	}
	return nil
}

// Helper functions

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value == "true" || value == "1" || value == "yes"
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var result int
	fmt.Sscanf(value, "%d", &result)
	if result == 0 {
		return defaultValue
	}
	return result
}

func getEnvFloatOrDefault(key string, defaultValue float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var result float64
	fmt.Sscanf(value, "%f", &result)
	if result == 0 {
		return defaultValue
	}
	return result
}

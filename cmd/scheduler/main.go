package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/k3suav/uav-monitor/pkg/config"
	"github.com/k3suav/uav-monitor/pkg/k8s"
	"github.com/k3suav/uav-monitor/pkg/scheduler"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm"
	schedulerConfig "github.com/k3suav/uav-monitor/pkg/scheduler/config"
	"github.com/k3suav/uav-monitor/pkg/scheduler/registry"
	"github.com/sirupsen/logrus"
)

const (
	version = "v0.1.0"
)

var log = logrus.New()

func main() {
	log.WithField("version", version).Info("Starting UAV Scheduler")

	// 1. 加载配置
	cfg := schedulerConfig.DefaultConfig()
	if err := cfg.Validate(); err != nil {
		log.WithError(err).Fatal("Invalid configuration")
	}

	log.WithFields(logrus.Fields{
		"schedulerName": cfg.SchedulerName,
		"algorithm":     cfg.AlgorithmName,
		"namespace":     cfg.Namespace,
	}).Info("Configuration loaded")

	// 2. 注册内置算法
	registerBuiltinAlgorithms(cfg)

	// 3. 获取要使用的算法
	algo, err := registry.Get(cfg.AlgorithmName)
	if err != nil {
		log.WithError(err).Fatalf("Algorithm '%s' not found. Available algorithms: %v",
			cfg.AlgorithmName, registry.List())
	}

	log.WithField("algorithm", algo.Name()).Info("Algorithm loaded")

	// 4. 创建 UAV Client（用于读取 CRD）
	uavConfig := config.DefaultConfig()
	uavConfig.Kubernetes.KubeconfigPath = cfg.KubeconfigPath
	uavConfig.Kubernetes.Namespace = cfg.Namespace

	uavClient, err := k8s.NewClient(uavConfig)
	if err != nil {
		log.WithError(err).Fatal("Failed to create UAV client")
	}

	log.Info("UAV client initialized")

	// 5. 创建调度器
	sched, err := scheduler.NewScheduler(cfg, algo, uavClient)
	if err != nil {
		log.WithError(err).Fatal("Failed to create scheduler")
	}

	log.Info("Scheduler initialized")

	// 6. 设置信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 7. 启动调度器
	errChan := make(chan error, 1)
	go func() {
		errChan <- sched.Run(ctx)
	}()

	// 8. 等待信号或错误
	select {
	case sig := <-sigChan:
		log.WithField("signal", sig).Info("Received shutdown signal")
		cancel()
	case err := <-errChan:
		if err != nil && err != context.Canceled {
			log.WithError(err).Error("Scheduler error")
		}
		cancel()
	}

	log.Info("Scheduler stopped")
}

// registerBuiltinAlgorithms 注册内置算法
func registerBuiltinAlgorithms(cfg *schedulerConfig.SchedulerConfig) {
	// 1. Distance-based 算法
	distanceAlgo := algorithm.NewDistanceBasedAlgorithm(
		cfg.AlgorithmParams.TargetLatitude,
		cfg.AlgorithmParams.TargetLongitude,
	)
	registry.Register(distanceAlgo)
	log.Debugf("Registered algorithm: %s", distanceAlgo.Name())

	// 2. Battery-aware 算法
	batteryAlgo := algorithm.NewBatteryAwareAlgorithm(cfg.AlgorithmParams.MinBattery)
	registry.Register(batteryAlgo)
	log.Debugf("Registered algorithm: %s", batteryAlgo.Name())

	// 3. Network-latency 算法
	networkAlgo := algorithm.NewNetworkLatencyAlgorithm(cfg.AlgorithmParams.MaxLatency)
	registry.Register(networkAlgo)
	log.Debugf("Registered algorithm: %s", networkAlgo.Name())

	// 4. Composite 算法（示例：组合 distance + battery）
	compositeAlgo := algorithm.NewCompositeAlgorithm(
		[]algorithm.SchedulingAlgorithm{distanceAlgo, batteryAlgo},
		[]float64{0.6, 0.4}, // 60% 距离权重，40% 电池权重
	)
	registry.Register(compositeAlgo)
	log.Debugf("Registered algorithm: %s", compositeAlgo.Name())

	log.WithField("algorithms", registry.List()).Info("Built-in algorithms registered")
}

// 扩展点：在这里可以加载外部插件算法
// func loadExternalAlgorithms() {
//     // 从配置文件或插件目录加载自定义算法
//     // plugin, err := plugin.Open("custom_algo.so")
//     // ...
// }

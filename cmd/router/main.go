package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/k3suav/uav-monitor/pkg/config"
	"github.com/k3suav/uav-monitor/pkg/k8s"
	"github.com/k3suav/uav-monitor/pkg/router"
	"github.com/k3suav/uav-monitor/pkg/router/algorithm"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	// 初始化日志
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// 获取节点名称
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		log.Fatal("NODE_NAME environment variable is required")
	}

	// 获取算法配置
	algorithmName := os.Getenv("ALGORITHM")
	if algorithmName == "" {
		algorithmName = "distance-based" // 默认使用距离算法
	}

	// 获取 API 端口
	apiPort := 8080
	if port := os.Getenv("API_PORT"); port != "" {
		// 可以添加端口解析逻辑
	}

	log.WithFields(logrus.Fields{
		"node":      nodeName,
		"algorithm": algorithmName,
		"port":      apiPort,
	}).Info("Starting UAV Router Agent")

	// 创建 Kubernetes 客户端
	k8sConfig, err := getK8sConfig()
	if err != nil {
		log.WithError(err).Fatal("Failed to get Kubernetes config")
	}

	k8sClientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		log.WithError(err).Fatal("Failed to create Kubernetes clientset")
	}

	// 创建 UAV Metrics 客户端
	uavConfig := config.DefaultConfig()
	uavClient, err := k8s.NewClient(uavConfig)
	if err != nil {
		log.WithError(err).Fatal("Failed to create UAV metrics client")
	}

	// 创建路由算法
	routingAlgorithm := createRoutingAlgorithm(algorithmName, log)

	// 创建 Router Agent
	routerAgent := router.NewRouterAgent(
		nodeName,
		k8sClientset,
		uavClient,
		routingAlgorithm,
		log,
	)

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动 Router Agent
	if err := routerAgent.Start(ctx); err != nil {
		log.WithError(err).Fatal("Failed to start router agent")
	}

	// 启动 HTTP API 服务器
	server := router.NewServer(routerAgent, apiPort, log)
	go func() {
		if err := server.Start(ctx); err != nil {
			log.WithError(err).Error("HTTP server stopped")
		}
	}()

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info("Shutting down router agent")
	cancel()
}

// getK8sConfig 获取 Kubernetes 配置
func getK8sConfig() (*rest.Config, error) {
	// 优先使用 in-cluster 配置
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// 否则使用 kubeconfig
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.Getenv("HOME") + "/.kube/config"
	}

	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

// createRoutingAlgorithm 创建路由算法实例
func createRoutingAlgorithm(name string, log *logrus.Logger) algorithm.RoutingAlgorithm {
	switch name {
	case "distance-based":
		log.Info("Using distance-based routing algorithm")
		return algorithm.NewDistanceBasedRouter(500.0) // 最大 500km

	case "battery-aware":
		log.Info("Using battery-aware routing algorithm")
		return algorithm.NewBatteryAwareRouter(20.0) // 最低 20% 电量

	case "composite":
		log.Info("Using composite routing algorithm")
		distanceAlgo := algorithm.NewDistanceBasedRouter(500.0)
		batteryAlgo := algorithm.NewBatteryAwareRouter(20.0)

		compositeAlgo, err := algorithm.NewCompositeRouter(
			[]algorithm.RoutingAlgorithm{distanceAlgo, batteryAlgo},
			[]float64{0.7, 0.3}, // 70% 距离权重, 30% 电量权重
		)
		if err != nil {
			log.WithError(err).Fatal("Failed to create composite algorithm")
		}
		return compositeAlgo

	default:
		log.WithField("algorithm", name).Warn("Unknown algorithm, using distance-based")
		return algorithm.NewDistanceBasedRouter(500.0)
	}
}

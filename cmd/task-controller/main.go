package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/k3suav/uav-monitor/pkg/config"
	"github.com/k3suav/uav-monitor/pkg/k8s"
	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/greed_nsgaii"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	log = logrus.New()
)

type TaskController struct {
	dynamicClient dynamic.Interface
	k8sClient     *kubernetes.Clientset
	uavClient     *k8s.Client
	namespace     string
	taskGVR       schema.GroupVersionResource
}

func main() {
	log.SetLevel(logrus.InfoLevel)
	log.Info("Starting UAV Task Controller")

	// 从环境变量加载配置
	namespace := getEnvOrDefault("NAMESPACE", "default")
	crdGroup := getEnvOrDefault("CRD_GROUP", "uav.k3s.io")
	crdVersion := getEnvOrDefault("CRD_VERSION", "v1")

	// 创建 K8s 客户端
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			log.WithError(err).Fatal("Failed to build k8s config")
		}
	}

	dynamicClient, err := dynamic.NewForConfig(k8sConfig)
	if err != nil {
		log.WithError(err).Fatal("Failed to create dynamic client")
	}

	k8sClient, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		log.WithError(err).Fatal("Failed to create k8s client")
	}

	// 创建 UAVMetrics 客户端（使用 v1alpha1 版本）
	uavMetricsConfig := &config.Config{
		Kubernetes: config.K8sConfig{
			Namespace:     namespace,
			CRDGroup:      crdGroup,
			CRDVersion:    "v1alpha1", // UAVMetrics uses v1alpha1
			RetryAttempts: 3,
			RetryDelay:    2 * time.Second,
		},
	}

	uavClient, err := k8s.NewClient(uavMetricsConfig)
	if err != nil {
		log.WithError(err).Fatal("Failed to create UAV client")
	}

	// 定义 UAVTask GVR
	taskGVR := schema.GroupVersionResource{
		Group:    crdGroup,
		Version:  crdVersion,
		Resource: "uavtasks",
	}

	controller := &TaskController{
		dynamicClient: dynamicClient,
		k8sClient:     k8sClient,
		uavClient:     uavClient,
		namespace:     namespace,
		taskGVR:       taskGVR,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("Received shutdown signal")
		cancel()
	}()

	// 启动控制器
	if err := controller.Run(ctx); err != nil {
		log.WithError(err).Fatal("Controller error")
	}
}

func (c *TaskController) Run(ctx context.Context) error {
	log.Info("Watching for UAVTask resources...")

	for {
		watcher, err := c.dynamicClient.Resource(c.taskGVR).
			Namespace(c.namespace).
			Watch(ctx, metav1.ListOptions{})
		if err != nil {
			log.WithError(err).Error("Failed to watch UAVTasks")
			time.Sleep(5 * time.Second)
			continue
		}

		for event := range watcher.ResultChan() {
			if event.Object == nil {
				continue
			}

			obj, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				if err := c.handleTask(ctx, obj); err != nil {
					log.WithError(err).WithField("task", obj.GetName()).Error("Failed to handle task")
				}
			}
		}

		select {
		case <-ctx.Done():
			return nil
		default:
			time.Sleep(1 * time.Second)
		}
	}
}

func (c *TaskController) handleTask(ctx context.Context, obj *unstructured.Unstructured) error {
	taskName := obj.GetName()

	// 获取 status.phase
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")

	// 如果已经处理过（有phase状态），跳过
	// 只处理新创建的任务（phase为空）或失败的任务
	if phase != "" && phase != "Failed" {
		return nil
	}

	log.WithField("task", taskName).Info("Processing UAVTask")

	// 更新状态为 Calculating
	if err := c.updateTaskStatus(ctx, taskName, "Calculating", "Running NSGA-II algorithm...", nil); err != nil {
		return err
	}

	// 获取 spec
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return fmt.Errorf("spec not found")
	}

	// 解析参数
	algorithm := getStringOrDefault(spec, "algorithm", "greed-nsgaii")
	targetCoverage := getFloatOrDefault(spec, "targetCoverage", 0.9)
	coverageRadius := getFloatOrDefault(spec, "coverageRadius", 500.0)
	taskType := getIntOrDefault(spec, "taskType", 0)

	log.WithFields(logrus.Fields{
		"algorithm":       algorithm,
		"targetCoverage":  targetCoverage,
		"coverageRadius":  coverageRadius,
		"taskType":        taskType,
	}).Info("Task parameters")

	// 获取所有 UAVMetrics
	metrics, err := c.uavClient.ListUAVMetrics(ctx)
	if err != nil {
		c.updateTaskStatus(ctx, taskName, "Failed", fmt.Sprintf("Failed to get metrics: %v", err), nil)
		return err
	}

	if len(metrics) == 0 {
		c.updateTaskStatus(ctx, taskName, "Failed", "No UAV nodes available", nil)
		return fmt.Errorf("no UAV nodes available")
	}

	log.WithField("nodeCount", len(metrics)).Info("Got UAV metrics")

	// 运行 NSGA-II 算法
	selectedNodes, coverageRatio, err := c.runNSGAII(metrics, targetCoverage, coverageRadius, greed_nsgaii.TaskType(taskType))
	if err != nil {
		c.updateTaskStatus(ctx, taskName, "Failed", fmt.Sprintf("Algorithm failed: %v", err), nil)
		return err
	}

	log.WithFields(logrus.Fields{
		"selectedNodes":  selectedNodes,
		"nodeCount":      len(selectedNodes),
		"coverageRatio":  coverageRatio,
	}).Info("NSGA-II calculation completed")

	// 更新状态为 Deploying
	statusData := map[string]interface{}{
		"selectedNodes": selectedNodes,
		"nodeCount":     len(selectedNodes),
		"coverageRatio": coverageRatio,
	}
	if err := c.updateTaskStatus(ctx, taskName, "Deploying", fmt.Sprintf("Creating %d pods...", len(selectedNodes)), statusData); err != nil {
		return err
	}

	// 获取 Pod 模板
	template, found, err := unstructured.NestedMap(spec, "template")
	if err != nil || !found {
		return fmt.Errorf("template not found")
	}

	// 创建 Pods
	createdPods := 0
	for i, nodeName := range selectedNodes {
		podName := fmt.Sprintf("%s-%d", taskName, i)
		if err := c.createPod(ctx, taskName, podName, nodeName, template); err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"pod":  podName,
				"node": nodeName,
			}).Error("Failed to create pod")
			continue
		}
		createdPods++
		log.WithFields(logrus.Fields{
			"pod":  podName,
			"node": nodeName,
		}).Info("Pod created and bound")
	}

	// 更新最终状态
	statusData["podCount"] = createdPods
	if err := c.updateTaskStatus(ctx, taskName, "Running", fmt.Sprintf("Successfully deployed %d pods", createdPods), statusData); err != nil {
		return err
	}

	log.WithField("task", taskName).Info("UAVTask deployed successfully")
	return nil
}

func (c *TaskController) runNSGAII(metrics []*models.UAVMetrics, targetCoverage, coverageRadius float64, taskType greed_nsgaii.TaskType) ([]string, float64, error) {
	// 配置 NSGA-II
	coverageConfig := &greed_nsgaii.CoverageConfig{
		TargetCoverageRatio: targetCoverage,
		CoverageRadius:      coverageRadius,
		GridDensity:         50,
	}

	nsga2Config := &greed_nsgaii.NSGA2Config{
		PopulationSize: 50,
		Generations:    30,
		CrossoverRate:  0.9,
		MutationRate:   0.1,
		CoverageConfig: coverageConfig,
		GreedySelector: greed_nsgaii.NewGreedySelector(coverageConfig),
	}

	// 将 UAVMetrics 转换为 NodeInfo
	scorer := greed_nsgaii.NewNodeScorer(taskType)
	allNodes := make([]*greed_nsgaii.NodeInfo, len(metrics))
	for i, m := range metrics {
		allNodes[i] = &greed_nsgaii.NodeInfo{
			Metrics: m,
			Score:   scorer.CalculateScore(m),
		}
	}

	// 初始化 GPS 转换器（使用第一个节点作为参考点）
	if len(allNodes) > 0 && allNodes[0].Metrics.GPS.Latitude != 0 && allNodes[0].Metrics.GPS.Longitude != 0 {
		converter := greed_nsgaii.NewGPSConverter(
			allNodes[0].Metrics.GPS.Latitude,
			allNodes[0].Metrics.GPS.Longitude,
		)
		for _, node := range allNodes {
			if node.Metrics.GPS.Latitude != 0 && node.Metrics.GPS.Longitude != 0 {
				node.XMeters, node.YMeters = converter.GPSToXY(
					node.Metrics.GPS.Latitude,
					node.Metrics.GPS.Longitude,
				)
			}
		}
	}

	// 运行 NSGA-II
	optimizer := greed_nsgaii.NewNSGA2Optimizer(nsga2Config, allNodes)
	result := optimizer.Optimize()

	// 获取最佳解
	if result.BestSolution == nil {
		return nil, 0, fmt.Errorf("no solution found")
	}

	selectedNodes := make([]string, 0)
	for i, selected := range result.BestSolution.Chromosome {
		if selected {
			selectedNodes = append(selectedNodes, allNodes[i].Metrics.NodeName)
		}
	}

	// 计算覆盖率（从目标值获取）
	coverage := result.BestSolution.Objectives[0] // 第一个目标是覆盖率

	return selectedNodes, -coverage, nil // 返回时取反（因为优化时是负值）
}

func (c *TaskController) createPod(ctx context.Context, taskName, podName, nodeName string, template map[string]interface{}) error {
	// 构建 Pod
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: c.namespace,
			Labels: map[string]string{
				"uav-task": taskName,
			},
		},
	}

	// 从 template 提取 spec
	if specMap, ok := template["spec"].(map[string]interface{}); ok {
		specJSON, err := json.Marshal(specMap)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(specJSON, &pod.Spec); err != nil {
			return err
		}
	}

	// 提取 metadata (labels, annotations)
	if metaMap, ok := template["metadata"].(map[string]interface{}); ok {
		if labels, ok := metaMap["labels"].(map[string]interface{}); ok {
			if pod.Labels == nil {
				pod.Labels = make(map[string]string)
			}
			for k, v := range labels {
				if str, ok := v.(string); ok {
					pod.Labels[k] = str
				}
			}
		}
		if annotations, ok := metaMap["annotations"].(map[string]interface{}); ok {
			pod.Annotations = make(map[string]string)
			for k, v := range annotations {
				if str, ok := v.(string); ok {
					pod.Annotations[k] = str
				}
			}
		}
	}

	// 设置 NodeName（直接绑定）
	pod.Spec.NodeName = nodeName

	// 创建 Pod
	_, err := c.k8sClient.CoreV1().Pods(c.namespace).Create(ctx, pod, metav1.CreateOptions{})
	return err
}

func (c *TaskController) updateTaskStatus(ctx context.Context, taskName, phase, message string, extraData map[string]interface{}) error {
	status := map[string]interface{}{
		"phase":          phase,
		"message":        message,
		"lastUpdateTime": time.Now().Format(time.RFC3339),
	}

	if extraData != nil {
		for k, v := range extraData {
			status[k] = v
		}
	}

	patch := map[string]interface{}{
		"status": status,
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return err
	}

	_, err = c.dynamicClient.Resource(c.taskGVR).
		Namespace(c.namespace).
		Patch(ctx, taskName, "application/merge-patch+json", patchBytes, metav1.PatchOptions{}, "status")

	return err
}

func getStringOrDefault(m map[string]interface{}, key, defaultValue string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return defaultValue
}

func getFloatOrDefault(m map[string]interface{}, key string, defaultValue float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	if v, ok := m[key].(int64); ok {
		return float64(v)
	}
	return defaultValue
}

func getIntOrDefault(m map[string]interface{}, key string, defaultValue int) int {
	if v, ok := m[key].(int64); ok {
		return int(v)
	}
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return defaultValue
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

package scheduler

import (
	"context"
	"sync"

	"github.com/k3suav/uav-monitor/pkg/k8s"
	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/greed_nsgaii"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// 需要覆盖率监控的算法列表
var coverageAlgorithms = map[string]bool{
	"greed-nsgaii":   true,
	"coverage-based": true,
}

// NeedsCoverageMonitor 判断算法是否需要覆盖率监控
func NeedsCoverageMonitor(algorithmName string) bool {
	return coverageAlgorithms[algorithmName]
}

// SchedulerWithMonitor 带覆盖率监控的调度器
// 只有使用覆盖率算法的 Deployment 才会被监控
type SchedulerWithMonitor struct {
	// 基础组件
	k8sClient kubernetes.Interface
	uavClient *k8s.Client

	// 自适应调度器（仅用于覆盖率算法）
	adaptiveScheduler *greed_nsgaii.AdaptiveScheduler
	monitor           *greed_nsgaii.CoverageMonitor
	monitorConfig     *greed_nsgaii.AdaptiveConfig

	// 跟踪哪些 Deployment 使用了覆盖率算法
	coverageDeployments map[string]*CoverageDeploymentInfo
	mu                  sync.RWMutex

	// 日志
	log *logrus.Entry

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
}

// CoverageDeploymentInfo 使用覆盖率算法的 Deployment 信息
type CoverageDeploymentInfo struct {
	DeploymentName string
	Namespace      string
	AlgorithmName  string   // greed-nsgaii 或 coverage-based
	SelectedNodes  []string
	BoundPods      map[string]string // podName -> nodeName
}

// NewSchedulerWithMonitor 创建带监控的调度器
func NewSchedulerWithMonitor(
	k8sClient kubernetes.Interface,
	uavClient *k8s.Client,
	monitorConfig *greed_nsgaii.AdaptiveConfig,
) *SchedulerWithMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	if monitorConfig == nil {
		monitorConfig = greed_nsgaii.DefaultAdaptiveConfig()
	}

	return &SchedulerWithMonitor{
		k8sClient:           k8sClient,
		uavClient:           uavClient,
		monitorConfig:       monitorConfig,
		coverageDeployments: make(map[string]*CoverageDeploymentInfo),
		log:                 logrus.WithField("component", "scheduler-monitor"),
		ctx:                 ctx,
		cancel:              cancel,
	}
}

// Start 启动监控（仅当有覆盖率算法的 Deployment 时才真正启动）
func (s *SchedulerWithMonitor) Start() {
	// 创建自适应调度器
	s.adaptiveScheduler = greed_nsgaii.NewAdaptiveScheduler(
		s.monitorConfig,
		greed_nsgaii.TaskTypeDefault,
	)

	// 创建监控器
	s.monitor = greed_nsgaii.NewCoverageMonitor(
		s.adaptiveScheduler,
		s, // 实现 MetricsProvider 接口
		s.monitorConfig.MonitorInterval,
	)

	// 启动监控
	s.monitor.Start()

	// 启动事件处理
	go s.eventLoop()

	s.log.Info("Coverage monitor started (waiting for coverage-based deployments)")
}

// Stop 停止
func (s *SchedulerWithMonitor) Stop() {
	s.cancel()
	if s.monitor != nil {
		s.monitor.Stop()
	}
}

// ListUAVMetrics 实现 MetricsProvider 接口
func (s *SchedulerWithMonitor) ListUAVMetrics(ctx context.Context) ([]*models.UAVMetrics, error) {
	return s.uavClient.ListUAVMetrics(ctx)
}

// OnPodScheduled Pod 调度完成后调用
// 只有使用覆盖率算法的 Pod 才会被跟踪
func (s *SchedulerWithMonitor) OnPodScheduled(pod *v1.Pod, nodeName string, algo algorithm.SchedulingAlgorithm) {
	algorithmName := algo.Name()

	// 检查是否是覆盖率算法
	if !NeedsCoverageMonitor(algorithmName) {
		// 不是覆盖率算法，不需要监控
		return
	}

	deploymentName := getDeploymentNameFromPod(pod)

	s.mu.Lock()
	defer s.mu.Unlock()

	// 获取或创建 Deployment 信息
	info, exists := s.coverageDeployments[deploymentName]
	if !exists {
		info = &CoverageDeploymentInfo{
			DeploymentName: deploymentName,
			Namespace:      pod.Namespace,
			AlgorithmName:  algorithmName,
			SelectedNodes:  []string{},
			BoundPods:      make(map[string]string),
		}
		s.coverageDeployments[deploymentName] = info

		// 首次，初始化监控
		s.initializeMonitoring(deploymentName, pod.Namespace)
	}

	// 记录节点
	if !contains(info.SelectedNodes, nodeName) {
		info.SelectedNodes = append(info.SelectedNodes, nodeName)
	}
	info.BoundPods[pod.Name] = nodeName

	// 更新自适应调度器的状态
	s.updateAdaptiveScheduler(deploymentName, info.SelectedNodes)

	s.log.WithFields(logrus.Fields{
		"deployment": deploymentName,
		"pod":        pod.Name,
		"node":       nodeName,
		"algorithm":  algorithmName,
	}).Info("Coverage deployment pod scheduled")
}

// initializeMonitoring 初始化监控
func (s *SchedulerWithMonitor) initializeMonitoring(deploymentName, namespace string) {
	// 获取当前 metrics
	metrics, err := s.uavClient.ListUAVMetrics(s.ctx)
	if err != nil {
		s.log.WithError(err).Error("Failed to get metrics for monitoring initialization")
		return
	}

	// 初始化自适应调度器
	s.mu.RUnlock() // 临时释放锁
	info := s.coverageDeployments[deploymentName]
	s.adaptiveScheduler.InitializeDeployment(deploymentName, info.SelectedNodes, metrics)
	s.mu.RLock()

	// 添加到监控列表
	s.monitor.Watch(deploymentName)

	s.log.WithField("deployment", deploymentName).Info("Started coverage monitoring for deployment")
}

// updateAdaptiveScheduler 更新自适应调度器状态
func (s *SchedulerWithMonitor) updateAdaptiveScheduler(deploymentName string, nodes []string) {
	metrics, err := s.uavClient.ListUAVMetrics(s.ctx)
	if err != nil {
		return
	}

	// 重新初始化（更新节点列表）
	s.adaptiveScheduler.InitializeDeployment(deploymentName, nodes, metrics)
}

// OnPodDeleted Pod 删除时调用
func (s *SchedulerWithMonitor) OnPodDeleted(pod *v1.Pod) {
	deploymentName := getDeploymentNameFromPod(pod)

	s.mu.Lock()
	defer s.mu.Unlock()

	info, exists := s.coverageDeployments[deploymentName]
	if !exists {
		return
	}

	// 移除 Pod 记录
	nodeName := info.BoundPods[pod.Name]
	delete(info.BoundPods, pod.Name)

	// 如果没有其他 Pod 在这个节点上，从已选节点中移除
	nodeStillUsed := false
	for _, n := range info.BoundPods {
		if n == nodeName {
			nodeStillUsed = true
			break
		}
	}

	if !nodeStillUsed {
		newNodes := []string{}
		for _, n := range info.SelectedNodes {
			if n != nodeName {
				newNodes = append(newNodes, n)
			}
		}
		info.SelectedNodes = newNodes

		// 通知自适应调度器节点被移除
		s.adaptiveScheduler.RemoveNode(deploymentName, nodeName)
	}

	// 如果 Deployment 没有 Pod 了，停止监控
	if len(info.BoundPods) == 0 {
		s.monitor.Unwatch(deploymentName)
		delete(s.coverageDeployments, deploymentName)
		s.log.WithField("deployment", deploymentName).Info("Stopped coverage monitoring (no pods)")
	}
}

// eventLoop 事件处理循环
func (s *SchedulerWithMonitor) eventLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return

		case event, ok := <-s.monitor.Events():
			if !ok {
				return
			}
			s.handleEvent(event)
		}
	}
}

// handleEvent 处理覆盖率事件
func (s *SchedulerWithMonitor) handleEvent(event *greed_nsgaii.CoverageEvent) {
	log := s.log.WithFields(logrus.Fields{
		"deployment": event.DeploymentName,
		"event":      event.EventType,
	})

	log.WithField("message", event.Message).Info("Coverage event")

	switch event.EventType {
	case greed_nsgaii.EventGreedyRequired:
		s.handleGreedyRequired(event)

	case greed_nsgaii.EventReplanRequired:
		s.handleReplanRequired(event)

	case greed_nsgaii.EventNodeOffline:
		log.Warn("Node offline detected")
	}
}

// handleGreedyRequired 处理贪心补充
func (s *SchedulerWithMonitor) handleGreedyRequired(event *greed_nsgaii.CoverageEvent) {
	metrics, err := s.uavClient.ListUAVMetrics(s.ctx)
	if err != nil {
		return
	}

	newNodes, err := s.adaptiveScheduler.ExecuteGreedyRepair(event.DeploymentName, metrics)
	if err != nil {
		s.log.WithError(err).Error("Greedy repair failed")
		return
	}

	if len(newNodes) == 0 {
		return
	}

	s.log.WithFields(logrus.Fields{
		"deployment": event.DeploymentName,
		"newNodes":   newNodes,
	}).Info("Greedy repair: new nodes selected")

	// 更新本地状态
	s.mu.Lock()
	if info, exists := s.coverageDeployments[event.DeploymentName]; exists {
		info.SelectedNodes = append(info.SelectedNodes, newNodes...)
	}
	s.mu.Unlock()

	// 注意：这里只是选出了节点，实际的 Pod 创建需要上层调度器处理
	// 或者等待 ReplicaSet 创建新的 Pending Pod
}

// handleReplanRequired 处理重规划
func (s *SchedulerWithMonitor) handleReplanRequired(event *greed_nsgaii.CoverageEvent) {
	metrics, err := s.uavClient.ListUAVMetrics(s.ctx)
	if err != nil {
		return
	}

	newNodes, result, err := s.adaptiveScheduler.ExecuteNSGA2Replan(event.DeploymentName, metrics)
	if err != nil {
		s.log.WithError(err).Error("NSGA-II replan failed")
		return
	}

	s.log.WithFields(logrus.Fields{
		"deployment": event.DeploymentName,
		"newNodes":   newNodes,
		"nodeCount":  len(newNodes),
	}).Info("NSGA-II replan completed")

	if result.BestSolution != nil {
		s.log.WithFields(logrus.Fields{
			"avgBattery": -result.BestSolution.Objectives[0],
			"avgLatency": result.BestSolution.Objectives[1],
		}).Info("Replan solution")
	}

	// 更新本地状态
	s.mu.Lock()
	if info, exists := s.coverageDeployments[event.DeploymentName]; exists {
		info.SelectedNodes = newNodes
	}
	s.mu.Unlock()

	// 触发 Pod 重调度（删除旧 Pod）
	s.triggerReschedule(event.DeploymentName)
}

// triggerReschedule 触发重调度
func (s *SchedulerWithMonitor) triggerReschedule(deploymentName string) {
	s.mu.RLock()
	info, exists := s.coverageDeployments[deploymentName]
	if !exists {
		s.mu.RUnlock()
		return
	}
	namespace := info.Namespace
	pods := make([]string, 0, len(info.BoundPods))
	for podName := range info.BoundPods {
		pods = append(pods, podName)
	}
	s.mu.RUnlock()

	// 删除旧 Pod（ReplicaSet 会自动重建）
	for _, podName := range pods {
		if err := s.k8sClient.CoreV1().Pods(namespace).Delete(s.ctx, podName, metav1.DeleteOptions{}); err != nil {
			s.log.WithError(err).WithField("pod", podName).Warn("Failed to delete pod")
		} else {
			s.log.WithField("pod", podName).Info("Pod deleted for reschedule")
		}
	}
}

// GetCoverageState 获取覆盖率状态
func (s *SchedulerWithMonitor) GetCoverageState(deploymentName string) *greed_nsgaii.CoverageState {
	return s.adaptiveScheduler.GetState(deploymentName)
}

// GetAllCoverageDeployments 获取所有使用覆盖率算法的 Deployment
func (s *SchedulerWithMonitor) GetAllCoverageDeployments() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	deployments := make([]string, 0, len(s.coverageDeployments))
	for name := range s.coverageDeployments {
		deployments = append(deployments, name)
	}
	return deployments
}

// IsMonitoringDeployment 检查 Deployment 是否正在被监控
func (s *SchedulerWithMonitor) IsMonitoringDeployment(deploymentName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.coverageDeployments[deploymentName]
	return exists
}

// 辅助函数

func getDeploymentNameFromPod(pod *v1.Pod) string {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			return owner.Name
		}
	}
	return pod.Name
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ============================================================
// 简化接口：集成到主调度器
// ============================================================

// CoverageMonitorCallback 覆盖率监控回调接口
type CoverageMonitorCallback interface {
	// OnPodScheduled Pod 调度完成后通知
	OnPodScheduled(pod *v1.Pod, nodeName string, algo algorithm.SchedulingAlgorithm)

	// OnPodDeleted Pod 删除后通知
	OnPodDeleted(pod *v1.Pod)
}

// Ensure SchedulerWithMonitor implements CoverageMonitorCallback
var _ CoverageMonitorCallback = (*SchedulerWithMonitor)(nil)

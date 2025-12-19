package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/k3suav/uav-monitor/pkg/k8s"
	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/greed_nsgaii"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// AdaptiveSchedulerIntegration 自适应调度器与 K8s 的集成
type AdaptiveSchedulerIntegration struct {
	// K8s 客户端
	k8sClient    kubernetes.Interface
	uavClient    *k8s.Client

	// 自适应调度器
	scheduler    *greed_nsgaii.AdaptiveScheduler
	monitor      *greed_nsgaii.CoverageMonitor

	// 配置
	config       *AdaptiveIntegrationConfig
	namespace    string

	// 状态
	mu           sync.RWMutex
	deployments  map[string]*DeploymentScheduleState

	// 日志
	log          *logrus.Entry

	// 控制
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// AdaptiveIntegrationConfig 集成配置
type AdaptiveIntegrationConfig struct {
	// 监控配置
	MonitorInterval       time.Duration  // 覆盖率检查间隔
	NodeTimeoutDuration   time.Duration  // 节点超时时间

	// 调度配置
	TargetCoverageRatio   float64        // 目标覆盖率 (0.9 = 90%)
	MinCoverageRatio      float64        // 最低可接受覆盖率
	MinorDropThreshold    float64        // 小幅下降阈值
	MajorDropThreshold    float64        // 大幅下降阈值

	// 算法配置
	CoverageRadius        float64        // 覆盖半径（米）
	GridDensity           int            // 网格密度
	TaskType              greed_nsgaii.TaskType

	// 执行配置
	AutoExecute           bool           // 是否自动执行调度动作
	MinReplanInterval     time.Duration  // 最小重规划间隔
	MinGreedyInterval     time.Duration  // 最小贪心补充间隔
}

// DefaultIntegrationConfig 默认配置
func DefaultIntegrationConfig() *AdaptiveIntegrationConfig {
	return &AdaptiveIntegrationConfig{
		MonitorInterval:       30 * time.Second,
		NodeTimeoutDuration:   60 * time.Second,
		TargetCoverageRatio:   0.90,
		MinCoverageRatio:      0.70,
		MinorDropThreshold:    0.10,
		MajorDropThreshold:    0.30,
		CoverageRadius:        500.0,
		GridDensity:           50,
		TaskType:              greed_nsgaii.TaskTypeDefault,
		AutoExecute:           true,
		MinReplanInterval:     5 * time.Minute,
		MinGreedyInterval:     1 * time.Minute,
	}
}

// DeploymentScheduleState Deployment 调度状态
type DeploymentScheduleState struct {
	DeploymentName   string
	Namespace        string
	SelectedNodes    []string
	BoundPods        map[string]string  // podName -> nodeName
	LastReplan       time.Time
	LastGreedy       time.Time
}

// NewAdaptiveSchedulerIntegration 创建集成实例
func NewAdaptiveSchedulerIntegration(
	k8sClient kubernetes.Interface,
	uavClient *k8s.Client,
	namespace string,
	config *AdaptiveIntegrationConfig,
) *AdaptiveSchedulerIntegration {
	if config == nil {
		config = DefaultIntegrationConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 创建自适应调度器配置
	adaptiveConfig := &greed_nsgaii.AdaptiveConfig{
		TargetCoverageRatio:   config.TargetCoverageRatio,
		MinCoverageRatio:      config.MinCoverageRatio,
		MinorDropThreshold:    config.MinorDropThreshold,
		MajorDropThreshold:    config.MajorDropThreshold,
		MonitorInterval:       config.MonitorInterval,
		NodeTimeoutDuration:   config.NodeTimeoutDuration,
		CoverageRadius:        config.CoverageRadius,
		GridDensity:           config.GridDensity,
	}

	scheduler := greed_nsgaii.NewAdaptiveScheduler(adaptiveConfig, config.TaskType)

	return &AdaptiveSchedulerIntegration{
		k8sClient:   k8sClient,
		uavClient:   uavClient,
		scheduler:   scheduler,
		config:      config,
		namespace:   namespace,
		deployments: make(map[string]*DeploymentScheduleState),
		log:         logrus.WithField("component", "adaptive-scheduler"),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// ============================================================
// 实现 MetricsProvider 接口
// ============================================================

// ListUAVMetrics 获取所有 UAVMetrics
func (a *AdaptiveSchedulerIntegration) ListUAVMetrics(ctx context.Context) ([]*models.UAVMetrics, error) {
	return a.uavClient.ListUAVMetrics(ctx)
}

// ============================================================
// 核心方法
// ============================================================

// Start 启动自适应调度
func (a *AdaptiveSchedulerIntegration) Start() error {
	a.log.Info("Starting adaptive scheduler integration")

	// 创建监控器
	a.monitor = greed_nsgaii.NewCoverageMonitor(a.scheduler, a, a.config.MonitorInterval)

	// 启动监控
	a.monitor.Start()

	// 启动事件处理
	a.wg.Add(1)
	go a.eventLoop()

	a.log.Info("Adaptive scheduler integration started")
	return nil
}

// Stop 停止
func (a *AdaptiveSchedulerIntegration) Stop() {
	a.log.Info("Stopping adaptive scheduler integration")
	a.cancel()
	if a.monitor != nil {
		a.monitor.Stop()
	}
	a.wg.Wait()
	a.log.Info("Adaptive scheduler integration stopped")
}

// RegisterDeployment 注册 Deployment 进行覆盖率跟踪
// 这是初始调度完成后调用的
func (a *AdaptiveSchedulerIntegration) RegisterDeployment(
	deploymentName string,
	namespace string,
	selectedNodes []string,
) error {
	a.log.WithFields(logrus.Fields{
		"deployment": deploymentName,
		"nodes":      selectedNodes,
	}).Info("Registering deployment for coverage monitoring")

	// 获取当前 metrics
	metrics, err := a.uavClient.ListUAVMetrics(a.ctx)
	if err != nil {
		return fmt.Errorf("failed to get metrics: %w", err)
	}

	// 初始化调度器跟踪
	if err := a.scheduler.InitializeDeployment(deploymentName, selectedNodes, metrics); err != nil {
		return fmt.Errorf("failed to initialize deployment: %w", err)
	}

	// 添加到监控列表
	a.monitor.Watch(deploymentName)

	// 保存状态
	a.mu.Lock()
	a.deployments[deploymentName] = &DeploymentScheduleState{
		DeploymentName: deploymentName,
		Namespace:      namespace,
		SelectedNodes:  selectedNodes,
		BoundPods:      make(map[string]string),
	}
	a.mu.Unlock()

	return nil
}

// UnregisterDeployment 取消注册
func (a *AdaptiveSchedulerIntegration) UnregisterDeployment(deploymentName string) {
	a.monitor.Unwatch(deploymentName)

	a.mu.Lock()
	delete(a.deployments, deploymentName)
	a.mu.Unlock()
}

// RecordPodBinding 记录 Pod 绑定（调度器绑定 Pod 后调用）
func (a *AdaptiveSchedulerIntegration) RecordPodBinding(deploymentName, podName, nodeName string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if state, exists := a.deployments[deploymentName]; exists {
		state.BoundPods[podName] = nodeName

		// 检查节点是否已在列表中，如果不在则添加
		nodeExists := false
		for _, n := range state.SelectedNodes {
			if n == nodeName {
				nodeExists = true
				break
			}
		}
		if !nodeExists {
			state.SelectedNodes = append(state.SelectedNodes, nodeName)

			// 通知自适应调度器更新节点列表
			go func() {
				metrics, err := a.uavClient.ListUAVMetrics(a.ctx)
				if err != nil {
					return
				}
				a.scheduler.InitializeDeployment(deploymentName, state.SelectedNodes, metrics)
			}()
		}
	}
}

// ============================================================
// 事件处理
// ============================================================

// eventLoop 事件处理循环
func (a *AdaptiveSchedulerIntegration) eventLoop() {
	defer a.wg.Done()

	for {
		select {
		case <-a.ctx.Done():
			return

		case event, ok := <-a.monitor.Events():
			if !ok {
				return
			}
			a.handleEvent(event)
		}
	}
}

// handleEvent 处理覆盖率事件
func (a *AdaptiveSchedulerIntegration) handleEvent(event *greed_nsgaii.CoverageEvent) {
	log := a.log.WithFields(logrus.Fields{
		"deployment": event.DeploymentName,
		"event":      event.EventType,
	})

	log.WithField("message", event.Message).Info("Coverage event received")

	if !a.config.AutoExecute {
		return
	}

	switch event.EventType {
	case greed_nsgaii.EventGreedyRequired:
		a.handleGreedyRequired(event)

	case greed_nsgaii.EventReplanRequired:
		a.handleReplanRequired(event)

	case greed_nsgaii.EventNodeOffline:
		log.Warn("Node went offline, coverage may be affected")
	}
}

// handleGreedyRequired 处理贪心补充请求
func (a *AdaptiveSchedulerIntegration) handleGreedyRequired(event *greed_nsgaii.CoverageEvent) {
	log := a.log.WithField("deployment", event.DeploymentName)

	// 检查间隔
	a.mu.Lock()
	state, exists := a.deployments[event.DeploymentName]
	if !exists {
		a.mu.Unlock()
		return
	}
	if time.Since(state.LastGreedy) < a.config.MinGreedyInterval {
		a.mu.Unlock()
		log.Debug("Skipping greedy repair, too soon since last repair")
		return
	}
	state.LastGreedy = time.Now()
	namespace := state.Namespace
	a.mu.Unlock()

	// 获取最新 metrics
	metrics, err := a.uavClient.ListUAVMetrics(a.ctx)
	if err != nil {
		log.WithError(err).Error("Failed to get metrics")
		return
	}

	// 执行贪心补充
	newNodes, err := a.scheduler.ExecuteGreedyRepair(event.DeploymentName, metrics)
	if err != nil {
		log.WithError(err).Error("Greedy repair failed")
		return
	}

	if len(newNodes) == 0 {
		log.Info("No new nodes needed")
		return
	}

	log.WithField("newNodes", newNodes).Info("Greedy repair completed")

	// 更新状态
	a.mu.Lock()
	if state, exists := a.deployments[event.DeploymentName]; exists {
		state.SelectedNodes = append(state.SelectedNodes, newNodes...)
	}
	a.mu.Unlock()

	// 为新节点创建 Pod（如果需要）
	// 这里假设有待调度的 Pod
	a.schedulePendingPods(event.DeploymentName, namespace, newNodes)
}

// handleReplanRequired 处理重规划请求
func (a *AdaptiveSchedulerIntegration) handleReplanRequired(event *greed_nsgaii.CoverageEvent) {
	log := a.log.WithField("deployment", event.DeploymentName)

	// 检查间隔
	a.mu.Lock()
	state, exists := a.deployments[event.DeploymentName]
	if !exists {
		a.mu.Unlock()
		return
	}
	if time.Since(state.LastReplan) < a.config.MinReplanInterval {
		a.mu.Unlock()
		log.Debug("Skipping replan, too soon since last replan")
		return
	}
	state.LastReplan = time.Now()
	namespace := state.Namespace
	a.mu.Unlock()

	// 获取最新 metrics
	metrics, err := a.uavClient.ListUAVMetrics(a.ctx)
	if err != nil {
		log.WithError(err).Error("Failed to get metrics")
		return
	}

	// 执行 NSGA-II 重规划
	newNodes, result, err := a.scheduler.ExecuteNSGA2Replan(event.DeploymentName, metrics)
	if err != nil {
		log.WithError(err).Error("NSGA-II replan failed")
		return
	}

	log.WithFields(logrus.Fields{
		"newNodes":  newNodes,
		"nodeCount": len(newNodes),
	}).Info("NSGA-II replan completed")

	if result.BestSolution != nil {
		log.WithFields(logrus.Fields{
			"avgBattery": -result.BestSolution.Objectives[0],
			"avgLatency": result.BestSolution.Objectives[1],
			"feasible":   result.BestSolution.IsFeasible,
		}).Info("Replan solution details")
	}

	// 更新状态
	a.mu.Lock()
	if state, exists := a.deployments[event.DeploymentName]; exists {
		state.SelectedNodes = newNodes
	}
	a.mu.Unlock()

	// 触发 Pod 重调度（删除旧 Pod，调度到新节点）
	a.triggerPodReschedule(event.DeploymentName, namespace, newNodes)
}

// schedulePendingPods 调度待处理的 Pod 到新节点
func (a *AdaptiveSchedulerIntegration) schedulePendingPods(deploymentName, namespace string, targetNodes []string) {
	// 获取该 Deployment 的待调度 Pod
	pods, err := a.k8sClient.CoreV1().Pods(namespace).List(a.ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=",  // 未绑定的 Pod
	})
	if err != nil {
		a.log.WithError(err).Error("Failed to list pending pods")
		return
	}

	// 筛选属于该 Deployment 的 Pod
	nodeIndex := 0
	for _, pod := range pods.Items {
		if !a.isPodBelongsToDeployment(&pod, deploymentName) {
			continue
		}

		if nodeIndex >= len(targetNodes) {
			break
		}

		// 绑定 Pod 到节点
		if err := a.bindPodToNode(&pod, targetNodes[nodeIndex]); err != nil {
			a.log.WithError(err).WithFields(logrus.Fields{
				"pod":  pod.Name,
				"node": targetNodes[nodeIndex],
			}).Error("Failed to bind pod")
			continue
		}

		a.log.WithFields(logrus.Fields{
			"pod":  pod.Name,
			"node": targetNodes[nodeIndex],
		}).Info("Pod bound to node")

		nodeIndex++
	}
}

// triggerPodReschedule 触发 Pod 重调度
func (a *AdaptiveSchedulerIntegration) triggerPodReschedule(deploymentName, namespace string, newNodes []string) {
	a.log.WithFields(logrus.Fields{
		"deployment": deploymentName,
		"newNodes":   newNodes,
	}).Info("Triggering pod reschedule")

	// 方案1: 删除旧 Pod，让 ReplicaSet 自动重建
	// 方案2: 直接修改 Pod 的 nodeName（需要先 evict）
	// 方案3: 使用 Pod Disruption Budget 逐步迁移

	// 这里使用方案1（简单）：删除现有 Pod
	a.mu.RLock()
	state, exists := a.deployments[deploymentName]
	if !exists {
		a.mu.RUnlock()
		return
	}
	boundPods := make(map[string]string)
	for k, v := range state.BoundPods {
		boundPods[k] = v
	}
	a.mu.RUnlock()

	// 删除旧 Pod（它们会被 ReplicaSet 重建）
	for podName := range boundPods {
		if err := a.k8sClient.CoreV1().Pods(namespace).Delete(a.ctx, podName, metav1.DeleteOptions{}); err != nil {
			a.log.WithError(err).WithField("pod", podName).Warn("Failed to delete pod for reschedule")
		} else {
			a.log.WithField("pod", podName).Info("Pod deleted for reschedule")
		}
	}

	// 清空已绑定 Pod 记录
	a.mu.Lock()
	if state, exists := a.deployments[deploymentName]; exists {
		state.BoundPods = make(map[string]string)
	}
	a.mu.Unlock()

	// 新 Pod 会被主调度器重新调度
}

// bindPodToNode 将 Pod 绑定到指定节点
func (a *AdaptiveSchedulerIntegration) bindPodToNode(pod *v1.Pod, nodeName string) error {
	binding := &v1.Binding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		Target: v1.ObjectReference{
			Kind: "Node",
			Name: nodeName,
		},
	}

	return a.k8sClient.CoreV1().Pods(pod.Namespace).Bind(a.ctx, binding, metav1.CreateOptions{})
}

// isPodBelongsToDeployment 检查 Pod 是否属于指定 Deployment
func (a *AdaptiveSchedulerIntegration) isPodBelongsToDeployment(pod *v1.Pod, deploymentName string) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			// ReplicaSet 名称格式: deployment-name-xxxxx
			// 简单匹配前缀
			if len(owner.Name) > len(deploymentName) && owner.Name[:len(deploymentName)] == deploymentName {
				return true
			}
		}
	}
	return false
}

// ============================================================
// 查询方法
// ============================================================

// GetDeploymentState 获取 Deployment 的覆盖率状态
func (a *AdaptiveSchedulerIntegration) GetDeploymentState(deploymentName string) *greed_nsgaii.CoverageState {
	return a.scheduler.GetState(deploymentName)
}

// GetAllDeploymentStates 获取所有 Deployment 的状态
func (a *AdaptiveSchedulerIntegration) GetAllDeploymentStates() map[string]*greed_nsgaii.CoverageState {
	a.mu.RLock()
	defer a.mu.RUnlock()

	states := make(map[string]*greed_nsgaii.CoverageState)
	for name := range a.deployments {
		states[name] = a.scheduler.GetState(name)
	}
	return states
}

// ManualCheck 手动触发覆盖率检查
func (a *AdaptiveSchedulerIntegration) ManualCheck(deploymentName string) (*greed_nsgaii.CoverageState, error) {
	metrics, err := a.uavClient.ListUAVMetrics(a.ctx)
	if err != nil {
		return nil, err
	}
	return a.scheduler.CheckAndDecide(deploymentName, metrics)
}

// ManualGreedyRepair 手动执行贪心补充
func (a *AdaptiveSchedulerIntegration) ManualGreedyRepair(deploymentName string) ([]string, error) {
	metrics, err := a.uavClient.ListUAVMetrics(a.ctx)
	if err != nil {
		return nil, err
	}
	return a.scheduler.ExecuteGreedyRepair(deploymentName, metrics)
}

// ManualReplan 手动执行重规划
func (a *AdaptiveSchedulerIntegration) ManualReplan(deploymentName string) ([]string, error) {
	metrics, err := a.uavClient.ListUAVMetrics(a.ctx)
	if err != nil {
		return nil, err
	}
	nodes, _, err := a.scheduler.ExecuteNSGA2Replan(deploymentName, metrics)
	return nodes, err
}

package greed_nsgaii

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// MetricsProvider 获取最新 UAVMetrics 的接口
// 在实际使用中，这个接口由 K8s Client 实现
type MetricsProvider interface {
	// ListUAVMetrics 获取所有节点的最新 UAVMetrics
	ListUAVMetrics(ctx context.Context) ([]*models.UAVMetrics, error)
}

// CoverageEvent 覆盖率事件
type CoverageEvent struct {
	DeploymentName string
	EventType      CoverageEventType
	State          *CoverageState
	Timestamp      time.Time
	Message        string
}

// CoverageEventType 事件类型
type CoverageEventType string

const (
	EventNodeOffline       CoverageEventType = "NodeOffline"       // 节点离线
	EventCoverageDropped   CoverageEventType = "CoverageDropped"   // 覆盖率下降
	EventGreedyRequired    CoverageEventType = "GreedyRequired"    // 需要贪心补充
	EventReplanRequired    CoverageEventType = "ReplanRequired"    // 需要重规划
	EventCoverageRecovered CoverageEventType = "CoverageRecovered" // 覆盖率恢复
)

// CoverageMonitor 覆盖率监控器
// 持续监控覆盖率变化，触发相应的调度动作
type CoverageMonitor struct {
	scheduler       *AdaptiveScheduler
	metricsProvider MetricsProvider

	// 监控配置
	checkInterval   time.Duration
	enabled         bool

	// 事件通道
	eventChan       chan *CoverageEvent

	// 控制
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	mu              sync.RWMutex

	// 监控的 Deployment 列表
	watchList       map[string]bool
}

// NewCoverageMonitor 创建覆盖率监控器
func NewCoverageMonitor(scheduler *AdaptiveScheduler, provider MetricsProvider, checkInterval time.Duration) *CoverageMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &CoverageMonitor{
		scheduler:       scheduler,
		metricsProvider: provider,
		checkInterval:   checkInterval,
		enabled:         false,
		eventChan:       make(chan *CoverageEvent, 100),
		ctx:             ctx,
		cancel:          cancel,
		watchList:       make(map[string]bool),
	}
}

// Watch 添加要监控的 Deployment
func (m *CoverageMonitor) Watch(deploymentName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watchList[deploymentName] = true
}

// Unwatch 移除监控
func (m *CoverageMonitor) Unwatch(deploymentName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.watchList, deploymentName)
}

// Events 获取事件通道（用于异步处理）
func (m *CoverageMonitor) Events() <-chan *CoverageEvent {
	return m.eventChan
}

// Start 启动监控
func (m *CoverageMonitor) Start() {
	m.mu.Lock()
	if m.enabled {
		m.mu.Unlock()
		return
	}
	m.enabled = true
	m.mu.Unlock()

	m.wg.Add(1)
	go m.monitorLoop()
}

// Stop 停止监控
func (m *CoverageMonitor) Stop() {
	m.mu.Lock()
	m.enabled = false
	m.mu.Unlock()

	m.cancel()
	m.wg.Wait()
	close(m.eventChan)
}

// monitorLoop 监控主循环
func (m *CoverageMonitor) monitorLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return

		case <-ticker.C:
			m.checkAllDeployments()
		}
	}
}

// checkAllDeployments 检查所有被监控的 Deployment
func (m *CoverageMonitor) checkAllDeployments() {
	// 获取最新的 metrics
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()

	metrics, err := m.metricsProvider.ListUAVMetrics(ctx)
	if err != nil {
		return
	}

	// 获取监控列表
	m.mu.RLock()
	deployments := make([]string, 0, len(m.watchList))
	for name := range m.watchList {
		deployments = append(deployments, name)
	}
	m.mu.RUnlock()

	// 检查每个 Deployment
	for _, deploymentName := range deployments {
		m.checkDeployment(deploymentName, metrics)
	}
}

// checkDeployment 检查单个 Deployment 的覆盖率
func (m *CoverageMonitor) checkDeployment(deploymentName string, metrics []*models.UAVMetrics) {
	// 获取上一次状态
	prevState := m.scheduler.GetState(deploymentName)
	if prevState == nil {
		return
	}

	// 检查并决定动作
	state, err := m.scheduler.CheckAndDecide(deploymentName, metrics)
	if err != nil {
		return
	}

	// 生成事件
	events := m.generateEvents(deploymentName, prevState, state)

	// 发送事件
	for _, event := range events {
		select {
		case m.eventChan <- event:
		default:
			// 通道满了，丢弃旧事件
		}
	}
}

// generateEvents 根据状态变化生成事件
func (m *CoverageMonitor) generateEvents(deploymentName string, prev, curr *CoverageState) []*CoverageEvent {
	events := []*CoverageEvent{}

	// 检查是否有节点离线
	if len(curr.OfflineNodes) > len(prev.OfflineNodes) {
		newOffline := findNewItems(prev.OfflineNodes, curr.OfflineNodes)
		for _, node := range newOffline {
			events = append(events, &CoverageEvent{
				DeploymentName: deploymentName,
				EventType:      EventNodeOffline,
				State:          curr,
				Timestamp:      time.Now(),
				Message:        fmt.Sprintf("Node %s went offline", node),
			})
		}
	}

	// 检查覆盖率变化
	if curr.CoverageDropRate > 0.05 { // 下降超过 5%
		events = append(events, &CoverageEvent{
			DeploymentName: deploymentName,
			EventType:      EventCoverageDropped,
			State:          curr,
			Timestamp:      time.Now(),
			Message:        fmt.Sprintf("Coverage dropped by %.1f%% (%.1f%% -> %.1f%%)",
				curr.CoverageDropRate*100, prev.CurrentCoverage*100, curr.CurrentCoverage*100),
		})
	}

	// 检查推荐动作
	switch curr.RecommendedAction {
	case ActionGreedy:
		events = append(events, &CoverageEvent{
			DeploymentName: deploymentName,
			EventType:      EventGreedyRequired,
			State:          curr,
			Timestamp:      time.Now(),
			Message:        fmt.Sprintf("Greedy repair recommended (coverage: %.1f%%)", curr.CurrentCoverage*100),
		})

	case ActionReplan:
		events = append(events, &CoverageEvent{
			DeploymentName: deploymentName,
			EventType:      EventReplanRequired,
			State:          curr,
			Timestamp:      time.Now(),
			Message:        fmt.Sprintf("NSGA-II replan recommended (coverage: %.1f%%, drop: %.1f%%)",
				curr.CurrentCoverage*100, curr.CoverageDropRate*100),
		})
	}

	// 检查覆盖率是否恢复
	if prev.CurrentCoverage < prev.TargetCoverage && curr.CurrentCoverage >= curr.TargetCoverage {
		events = append(events, &CoverageEvent{
			DeploymentName: deploymentName,
			EventType:      EventCoverageRecovered,
			State:          curr,
			Timestamp:      time.Now(),
			Message:        fmt.Sprintf("Coverage recovered to %.1f%%", curr.CurrentCoverage*100),
		})
	}

	return events
}

// findNewItems 找出新增的项目
func findNewItems(old, new []string) []string {
	oldSet := make(map[string]bool)
	for _, item := range old {
		oldSet[item] = true
	}

	newItems := []string{}
	for _, item := range new {
		if !oldSet[item] {
			newItems = append(newItems, item)
		}
	}
	return newItems
}

// ============================================================
// 自动调度控制器 - 结合监控和自动执行
// ============================================================

// AutoSchedulerConfig 自动调度配置
type AutoSchedulerConfig struct {
	// 是否自动执行调度动作
	AutoExecute       bool

	// 是否自动绑定到 K8s（需要提供 PodBinder）
	AutoBind          bool

	// 执行间隔（避免频繁重规划）
	MinReplanInterval time.Duration
	MinGreedyInterval time.Duration
}

// PodBinder Pod 绑定接口
type PodBinder interface {
	// BindPodToNode 将 Pod 绑定到指定节点
	BindPodToNode(ctx context.Context, podName, namespace, nodeName string) error

	// GetPendingPods 获取待调度的 Pod
	GetPendingPods(ctx context.Context, deploymentName string) ([]string, error)
}

// AutoScheduler 自动调度控制器
type AutoScheduler struct {
	monitor         *CoverageMonitor
	scheduler       *AdaptiveScheduler
	metricsProvider MetricsProvider
	podBinder       PodBinder
	config          *AutoSchedulerConfig

	// 上次执行时间
	lastReplan      map[string]time.Time
	lastGreedy      map[string]time.Time
	mu              sync.Mutex

	// 控制
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// NewAutoScheduler 创建自动调度控制器
func NewAutoScheduler(
	scheduler *AdaptiveScheduler,
	provider MetricsProvider,
	binder PodBinder,
	config *AutoSchedulerConfig,
	checkInterval time.Duration,
) *AutoScheduler {
	ctx, cancel := context.WithCancel(context.Background())

	monitor := NewCoverageMonitor(scheduler, provider, checkInterval)

	return &AutoScheduler{
		monitor:         monitor,
		scheduler:       scheduler,
		metricsProvider: provider,
		podBinder:       binder,
		config:          config,
		lastReplan:      make(map[string]time.Time),
		lastGreedy:      make(map[string]time.Time),
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Watch 添加要监控的 Deployment
func (a *AutoScheduler) Watch(deploymentName string, initialNodes []string) error {
	// 获取当前 metrics
	metrics, err := a.metricsProvider.ListUAVMetrics(a.ctx)
	if err != nil {
		return err
	}

	// 初始化 Deployment 跟踪
	if err := a.scheduler.InitializeDeployment(deploymentName, initialNodes, metrics); err != nil {
		return err
	}

	// 添加到监控列表
	a.monitor.Watch(deploymentName)
	return nil
}

// Start 启动自动调度
func (a *AutoScheduler) Start() {
	// 启动监控
	a.monitor.Start()

	// 启动事件处理器
	a.wg.Add(1)
	go a.eventHandler()
}

// Stop 停止自动调度
func (a *AutoScheduler) Stop() {
	a.cancel()
	a.monitor.Stop()
	a.wg.Wait()
}

// eventHandler 事件处理器
func (a *AutoScheduler) eventHandler() {
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

// handleEvent 处理事件
func (a *AutoScheduler) handleEvent(event *CoverageEvent) {
	if !a.config.AutoExecute {
		// 只记录日志，不自动执行
		fmt.Printf("[CoverageMonitor] %s: %s\n", event.EventType, event.Message)
		return
	}

	switch event.EventType {
	case EventGreedyRequired:
		a.executeGreedy(event.DeploymentName)

	case EventReplanRequired:
		a.executeReplan(event.DeploymentName)
	}
}

// executeGreedy 执行贪心补充
func (a *AutoScheduler) executeGreedy(deploymentName string) {
	a.mu.Lock()
	lastTime := a.lastGreedy[deploymentName]
	if time.Since(lastTime) < a.config.MinGreedyInterval {
		a.mu.Unlock()
		return // 间隔太短，跳过
	}
	a.lastGreedy[deploymentName] = time.Now()
	a.mu.Unlock()

	// 获取最新 metrics
	metrics, err := a.metricsProvider.ListUAVMetrics(a.ctx)
	if err != nil {
		return
	}

	// 执行贪心补充
	newNodes, err := a.scheduler.ExecuteGreedyRepair(deploymentName, metrics)
	if err != nil {
		return
	}

	fmt.Printf("[AutoScheduler] Greedy repair completed, added nodes: %v\n", newNodes)

	// 如果需要自动绑定
	if a.config.AutoBind && a.podBinder != nil {
		a.bindNewNodes(deploymentName, newNodes)
	}
}

// executeReplan 执行重规划
func (a *AutoScheduler) executeReplan(deploymentName string) {
	a.mu.Lock()
	lastTime := a.lastReplan[deploymentName]
	if time.Since(lastTime) < a.config.MinReplanInterval {
		a.mu.Unlock()
		return // 间隔太短，跳过
	}
	a.lastReplan[deploymentName] = time.Now()
	a.mu.Unlock()

	// 获取最新 metrics
	metrics, err := a.metricsProvider.ListUAVMetrics(a.ctx)
	if err != nil {
		return
	}

	// 执行 NSGA-II 重规划
	newNodes, _, err := a.scheduler.ExecuteNSGA2Replan(deploymentName, metrics)
	if err != nil {
		return
	}

	fmt.Printf("[AutoScheduler] NSGA-II replan completed, selected nodes: %v\n", newNodes)

	// 如果需要自动绑定
	if a.config.AutoBind && a.podBinder != nil {
		// 重规划需要重新绑定所有 Pod
		a.rebindAllPods(deploymentName, newNodes)
	}
}

// bindNewNodes 绑定新节点
func (a *AutoScheduler) bindNewNodes(deploymentName string, newNodes []string) {
	// 获取待调度的 Pod
	pendingPods, err := a.podBinder.GetPendingPods(a.ctx, deploymentName)
	if err != nil {
		return
	}

	// 绑定 Pod 到新节点
	for i, podName := range pendingPods {
		if i >= len(newNodes) {
			break
		}
		if err := a.podBinder.BindPodToNode(a.ctx, podName, "default", newNodes[i]); err != nil {
			fmt.Printf("[AutoScheduler] Failed to bind pod %s to node %s: %v\n", podName, newNodes[i], err)
		}
	}
}

// rebindAllPods 重新绑定所有 Pod（用于重规划）
func (a *AutoScheduler) rebindAllPods(deploymentName string, newNodes []string) {
	// 这里需要更复杂的逻辑：
	// 1. 删除旧的 Pod
	// 2. 等待新的 Pod 被创建
	// 3. 绑定新 Pod 到新节点
	//
	// 实际实现中，可能需要与 Deployment Controller 协作
	fmt.Printf("[AutoScheduler] Rebind all pods to: %v (requires implementation)\n", newNodes)
}

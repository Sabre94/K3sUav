package algorithm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
	v1 "k8s.io/api/core/v1"
)

// CoverageBasedAlgorithm 基于覆盖率的调度算法
// 简化版：为同一个 Deployment 的多个 Pod 选择不同的节点，以实现最大覆盖
type CoverageBasedAlgorithm struct {
	CoverageRequirement float64 // 覆盖率要求（例如 90）
	CoverageRadius      float64 // 每个节点的覆盖半径（km）

	// 状态缓存：记录每个 Deployment 已选择的节点
	deploymentCoverage map[string]*DeploymentCoverage
	mu                 sync.RWMutex

	// 每个 Deployment 的调度锁（贪心算法：确保串行调度）
	deploymentLocks map[string]*sync.Mutex
	locksmu         sync.Mutex
}

// DeploymentCoverage Deployment 的覆盖信息
type DeploymentCoverage struct {
	SelectedNodes   []string  // 已选择的节点列表
	CurrentCoverage float64   // 当前覆盖率
	LastUpdate      time.Time // 最后更新时间
}

// NewCoverageBasedAlgorithm 创建基于覆盖率的算法
func NewCoverageBasedAlgorithm(requirement, radius float64) *CoverageBasedAlgorithm {
	return &CoverageBasedAlgorithm{
		CoverageRequirement: requirement,
		CoverageRadius:      radius,
		deploymentCoverage:  make(map[string]*DeploymentCoverage),
		deploymentLocks:     make(map[string]*sync.Mutex),
	}
}

func (a *CoverageBasedAlgorithm) Name() string {
	return "coverage-based"
}

func (a *CoverageBasedAlgorithm) Filter(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]*models.UAVMetrics, error) {
	// 不做硬性过滤
	return metrics, nil
}

func (a *CoverageBasedAlgorithm) Score(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]NodeScore, error) {
	// 1. 获取 Pod 所属的 Deployment/ReplicaSet
	deploymentName := getDeploymentName(pod)

	// 2. 获取该 Deployment 已选择的节点（只读，不加写锁）
	a.mu.RLock()
	coverage, exists := a.deploymentCoverage[deploymentName]
	if !exists {
		coverage = &DeploymentCoverage{
			SelectedNodes:   []string{},
			CurrentCoverage: 0.0,
			LastUpdate:      time.Now(),
		}
	}

	// 复制已选节点列表（避免并发修改）
	selectedNodes := make([]string, len(coverage.SelectedNodes))
	copy(selectedNodes, coverage.SelectedNodes)
	currentCoverage := coverage.CurrentCoverage
	a.mu.RUnlock()

	// 3. 为每个节点计算增量覆盖得分（基于已选节点）
	scores := []NodeScore{}
	selectedNodesMetrics := a.getMetricsForNodes(selectedNodes, metrics)

	for _, m := range metrics {
		// 跳过已选择的节点（避免重复调度）
		if contains(selectedNodes, m.NodeName) {
			scores = append(scores, NodeScore{
				NodeName: m.NodeName,
				Score:    0.0, // 已选择的节点分数为0
				Reason:   fmt.Sprintf("already selected for coverage (total: %d nodes)", len(selectedNodes)),
			})
			continue
		}

		// 计算选择该节点后的增量覆盖
		incrementalCoverage := a.calculateIncrementalCoverage(m, selectedNodesMetrics)

		scores = append(scores, NodeScore{
			NodeName: m.NodeName,
			Score:    incrementalCoverage * 100, // 增量越大分数越高
			Reason: fmt.Sprintf("incremental coverage: %.2f%%, current: %.2f%% (%d/%d nodes)",
				incrementalCoverage, currentCoverage, len(selectedNodes), len(metrics)),
		})
	}

	// 注意：不在这里更新缓存！
	// 缓存更新由 RecordBinding() 方法负责，在 Pod 真正绑定后调用

	return scores, nil
}

// calculateIncrementalCoverage 计算新增覆盖面积百分比
// 简化算法：基于节点间距离计算覆盖重叠
func (a *CoverageBasedAlgorithm) calculateIncrementalCoverage(
	newNode *models.UAVMetrics,
	existingNodes []*models.UAVMetrics) float64 {

	// 如果没有已选择的节点，这是第一个节点
	if len(existingNodes) == 0 {
		// 单个节点的覆盖面积占总面积的百分比
		// 简化计算：假设总区域是 100km x 100km
		totalArea := 100.0 * 100.0        // 10000 km²
		nodeArea := 3.14159 * a.CoverageRadius * a.CoverageRadius
		return (nodeArea / totalArea) * 100.0
	}

	// 计算新节点与已有节点的最小距离
	minDistance := 999999.0
	for _, existing := range existingNodes {
		distance := CalculateDistance(
			newNode.GPS.Latitude, newNode.GPS.Longitude,
			existing.GPS.Latitude, existing.GPS.Longitude,
		)
		if distance < minDistance {
			minDistance = distance
		}
	}

	// 基于最小距离计算增量覆盖
	// 距离越远，增量覆盖越大
	totalArea := 100.0 * 100.0
	nodeArea := 3.14159 * a.CoverageRadius * a.CoverageRadius
	baseCoverage := (nodeArea / totalArea) * 100.0

	// 如果距离大于 2 倍覆盖半径，几乎没有重叠
	if minDistance >= 2*a.CoverageRadius {
		return baseCoverage
	}

	// 如果距离很近，有较大重叠，增量覆盖减少
	overlapRatio := 1.0 - (minDistance / (2 * a.CoverageRadius))
	incrementalCoverage := baseCoverage * (1.0 - overlapRatio*0.5)

	return incrementalCoverage
}

// getDeploymentName 从 Pod 的 OwnerReferences 中获取 Deployment/ReplicaSet 名称
func getDeploymentName(pod *v1.Pod) string {
	// 从 Pod 的 OwnerReferences 中获取 Deployment/ReplicaSet 名称
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			// ReplicaSet 名称格式: deployment-name-xxxxx
			return owner.Name
		}
	}
	// 如果没有 owner，使用 Pod 名称
	return pod.Name
}

// getMetricsForNodes 获取指定节点的 metrics
func (a *CoverageBasedAlgorithm) getMetricsForNodes(nodeNames []string, allMetrics []*models.UAVMetrics) []*models.UAVMetrics {
	result := []*models.UAVMetrics{}
	for _, name := range nodeNames {
		for _, m := range allMetrics {
			if m.NodeName == name {
				result = append(result, m)
				break
			}
		}
	}
	return result
}

// contains 检查切片中是否包含某个元素
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// RecordBinding 记录 Pod 绑定到节点（贪心算法的关键：绑定后才更新缓存）
func (a *CoverageBasedAlgorithm) RecordBinding(pod *v1.Pod, nodeName string, incrementalCoverage float64) {
	deploymentName := getDeploymentName(pod)

	a.mu.Lock()
	defer a.mu.Unlock()

	coverage, exists := a.deploymentCoverage[deploymentName]
	if !exists {
		coverage = &DeploymentCoverage{
			SelectedNodes:   []string{},
			CurrentCoverage: 0.0,
			LastUpdate:      time.Now(),
		}
		a.deploymentCoverage[deploymentName] = coverage
	}

	// 检查节点是否已存在（避免重复添加）
	if !contains(coverage.SelectedNodes, nodeName) {
		coverage.SelectedNodes = append(coverage.SelectedNodes, nodeName)
		coverage.CurrentCoverage += incrementalCoverage
		coverage.LastUpdate = time.Now()
	}
}

// CleanupDeployment 清理指定 Deployment 的覆盖缓存
func (a *CoverageBasedAlgorithm) CleanupDeployment(deploymentName string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.deploymentCoverage, deploymentName)
}

// GetCoverageInfo 获取指定 Deployment 的覆盖信息（用于调试）
func (a *CoverageBasedAlgorithm) GetCoverageInfo(deploymentName string) *DeploymentCoverage {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.deploymentCoverage[deploymentName]
}

// LockDeployment 锁定 Deployment 的调度（贪心算法：确保串行调度）
func (a *CoverageBasedAlgorithm) LockDeployment(deploymentName string) {
	a.locksmu.Lock()
	lock, exists := a.deploymentLocks[deploymentName]
	if !exists {
		lock = &sync.Mutex{}
		a.deploymentLocks[deploymentName] = lock
	}
	a.locksmu.Unlock()

	// 锁定该 Deployment 的调度
	lock.Lock()
}

// UnlockDeployment 解锁 Deployment 的调度
func (a *CoverageBasedAlgorithm) UnlockDeployment(deploymentName string) {
	a.locksmu.Lock()
	lock, exists := a.deploymentLocks[deploymentName]
	a.locksmu.Unlock()

	if exists {
		lock.Unlock()
	}
}

package registry

import (
	"fmt"
	"sync"

	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm"
)

// AlgorithmRegistry 算法注册表
type AlgorithmRegistry struct {
	algorithms map[string]algorithm.SchedulingAlgorithm
	mu         sync.RWMutex
}

var (
	// 全局注册表
	globalRegistry = &AlgorithmRegistry{
		algorithms: make(map[string]algorithm.SchedulingAlgorithm),
	}
)

// Register 注册算法到全局注册表
func Register(algo algorithm.SchedulingAlgorithm) {
	globalRegistry.Register(algo)
}

// Get 从全局注册表获取算法
func Get(name string) (algorithm.SchedulingAlgorithm, error) {
	return globalRegistry.Get(name)
}

// List 列出全局注册表中的所有算法
func List() []string {
	return globalRegistry.List()
}

// Register 注册算法
func (r *AlgorithmRegistry) Register(algo algorithm.SchedulingAlgorithm) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.algorithms[algo.Name()] = algo
}

// Get 获取算法
func (r *AlgorithmRegistry) Get(name string) (algorithm.SchedulingAlgorithm, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	algo, ok := r.algorithms[name]
	if !ok {
		return nil, fmt.Errorf("algorithm '%s' not found in registry", name)
	}
	return algo, nil
}

// List 列出所有已注册的算法名称
func (r *AlgorithmRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.algorithms))
	for name := range r.algorithms {
		names = append(names, name)
	}
	return names
}

// Clear 清空注册表（主要用于测试）
func (r *AlgorithmRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.algorithms = make(map[string]algorithm.SchedulingAlgorithm)
}

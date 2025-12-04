# GREED + NSGA-II 混合调度算法

## 概述

GREED_NSGAII 是一个两阶段混合优化算法，专门为 UAV（无人机）集群调度设计。该算法结合了贪心算法的快速性和 NSGA-II 多目标进化算法的全局优化能力。

## 算法架构

### 两阶段设计

#### 阶段 1: Greedy Phase（贪心阶段）
- **目标**: 快速选择满足覆盖率约束的节点子集
- **策略**: 每次选择 `(增量覆盖面积 × 节点得分)` 最大的节点
- **特点**:
  - 快速收敛
  - 保证满足覆盖率要求
  - 适用于在线实时调度

#### 阶段 2: NSGA-II Phase（多目标优化阶段）
- **目标**: 在满足覆盖率约束下，同时优化4个目标
- **优化目标**:
  1. 最大化平均电量（目标值：负平均电量）
  2. 最小化平均网络延迟
  3. 最小化平均资源利用率（CPU + 内存）
  4. 最小化使用的 UAV 数量
- **输出**: Pareto 前沿（非支配解集）

## 核心组件

### 1. GPS 坐标转换 (`gps_utils.go`)
- **GPSConverter**: 将 GPS 坐标转换为相对 XY 坐标（米）
- **HaversineDistance**: 计算两个 GPS 点之间的距离
- 支持正向和反向转换

### 2. 节点评分器 (`scorer.go`)
- **任务类型权重**:
  - `emergency`: 紧急任务 (延迟 50%, 电量 40%, 利用率 10%)
  - `sustain`: 持续任务 (电量 50%, 利用率 30%, 延迟 20%)
  - `compute`: 计算任务 (利用率 40%, 延迟 40%, 电量 20%)
  - `default`: 默认任务 (均衡分配 33/33/34%)

- **评分维度**:
  - 电量百分比（0-100%）
  - 网络延迟（10-150ms）
  - 资源利用率（CPU + 内存平均值）

### 3. 覆盖面积计算 (`coverage.go`)
- **网格采样法**: 将区域划分为网格，统计被覆盖的网格点数量
- **增量计算**: 高效计算添加新节点后的增量覆盖面积
- **自适应边界**: 根据节点分布自动计算绘图区域

### 4. 贪心选择器 (`greedy.go`)
- **GreedySelector**: 实现贪心节点选择逻辑
- **增益函数**: `gain = incremental_area × node_score`
- **终止条件**: 覆盖率达到目标或无可选节点

### 5. NSGA-II 优化器 (`nsga2.go`)
- **遗传算法组件**:
  - 交叉操作（单点交叉）
  - 变异操作（位翻转）
  - 修复机制（贪心修复以满足约束）
- **核心算法**:
  - 快速非支配排序（Fast Non-Dominated Sort）
  - 拥挤度距离计算（Crowding Distance）
  - 约束支配关系（Constrained Dominance）

### 6. 调度算法接口 (`algorithm.go`)
- **GreedNSGAIIAlgorithm**: 实现 K8s 调度器接口
- **有状态调度**: 维护每个 Deployment 的选择历史
- **线程安全**: 使用互斥锁保护状态缓存

## 使用方式

### 1. 通过 Pod Annotation 配置

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  annotations:
    uav.scheduler/algorithm: "greed-nsgaii"
    uav.scheduler/task-type: "emergency"       # emergency/sustain/compute/default
    uav.scheduler/target-coverage: "0.9"       # 0.0-1.0 (90% 覆盖率)
    uav.scheduler/coverage-radius: "200.0"     # 覆盖半径（米）
spec:
  schedulerName: uav-scheduler
  ...
```

### 2. 编程方式创建

```go
import "github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/greed_nsgaii"

// 创建算法实例
algo := greed_nsgaii.NewGreedNSGAIIAlgorithm(
    greed_nsgaii.TaskTypeEmergency,  // 任务类型
    0.9,                               // 目标覆盖率 (90%)
    200.0,                             // 覆盖半径 (200米)
)

// 使用算法进行调度
scores, err := algo.Score(ctx, pod, metrics)
```

### 3. 运行 NSGA-II 离线优化

```go
// 运行完整的 NSGA-II 优化
result := algo.RunNSGA2Optimization(allMetrics)

// 获取 Pareto 前沿
paretoFront := result.ParetoFront

// 获取推荐解
bestSolution := result.BestSolution
```

## 配置参数

### 覆盖率配置
- `TargetCoverageRatio`: 目标覆盖率（0.0-1.0），默认 0.9
- `CoverageRadius`: 节点覆盖半径（米），默认 200m
- `GridDensity`: 网格密度（用于覆盖计算），默认 50×50

### NSGA-II 配置
- `PopulationSize`: 种群大小，默认 50
- `Generations`: 进化代数，默认 30
- `CrossoverRate`: 交叉概率，默认 0.9
- `MutationRate`: 变异概率，默认 0.1

## 算法特性

### 优势
1. **快速响应**: 贪心阶段提供快速的在线调度决策
2. **全局优化**: NSGA-II 提供多目标优化的最优解集
3. **约束满足**: 保证满足覆盖率约束
4. **任务适配**: 支持不同任务类型的权重配置
5. **状态感知**: 考虑已选节点，避免重复选择

### 适用场景
- 需要覆盖特定区域的 UAV 任务部署
- 对延迟、电量、资源利用率有多目标要求的场景
- 紧急救援、持续监控、计算密集型任务

### 性能特点
- **时间复杂度**（Greedy 阶段）: O(n²) - n 为节点数
- **空间复杂度**: O(n + g²) - g 为网格密度
- **调度延迟**: < 100ms（典型场景，26 个节点）

## 文件结构

```
greed_nsgaii/
├── README.md           # 本文档
├── algorithm.go        # 调度算法接口实现
├── coverage.go         # 覆盖面积计算
├── gps_utils.go        # GPS 坐标转换工具
├── greedy.go           # 贪心选择算法
├── nsga2.go            # NSGA-II 多目标优化
├── scorer.go           # 节点评分器
└── types.go            # 数据结构定义
```

## 示例

### 调度一个紧急任务 Deployment

```bash
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: emergency-rescue
spec:
  replicas: 5
  selector:
    matchLabels:
      app: rescue
  template:
    metadata:
      labels:
        app: rescue
      annotations:
        uav.scheduler/algorithm: "greed-nsgaii"
        uav.scheduler/task-type: "emergency"
        uav.scheduler/target-coverage: "0.95"
        uav.scheduler/coverage-radius: "250"
    spec:
      schedulerName: uav-scheduler
      containers:
      - name: rescue-app
        image: rescue:v1
EOF
```

### 查看调度结果

算法会按以下策略调度 5 个 Pod：
1. 第一个 Pod: 选择综合得分最高的节点
2. 后续 Pod: 依次选择能最大化增量覆盖的节点
3. 直到覆盖率达到 95% 或所有 Pod 都调度完成

## 扩展与定制

### 添加新的任务类型

在 `types.go` 中添加新的任务类型：

```go
const (
    TaskTypeCustom TaskType = "custom"
)

func GetTaskWeights(taskType TaskType) TaskWeights {
    switch taskType {
    case TaskTypeCustom:
        return TaskWeights{Battery: 0.6, Latency: 0.3, Util: 0.1}
    // ...
    }
}
```

### 自定义评分函数

修改 `scorer.go` 中的 `CalculateScore` 方法，添加新的评分维度。

### 调整 NSGA-II 参数

在创建算法时自定义 NSGA-II 配置：

```go
algo := greed_nsgaii.NewGreedNSGAIIAlgorithm(taskType, coverage, radius)
algo.nsga2Config.PopulationSize = 100  // 增大种群
algo.nsga2Config.Generations = 50      // 增加迭代代数
```

## 性能优化建议

1. **网格密度**: 根据区域大小调整 `GridDensity`（密度越大越精确，但计算越慢）
2. **NSGA-II 参数**: 根据实时性要求调整种群大小和代数
3. **缓存复用**: 相同配置的 Deployment 会共享算法实例
4. **并发控制**: 同一 Deployment 的调度会串行化，避免竞争

## 参考文献

1. Deb, K., et al. (2002). "A fast and elitist multiobjective genetic algorithm: NSGA-II"
2. UAV Coverage Path Planning: Survey and Future Directions
3. Multi-UAV Task Assignment with Constraints

## 作者

K3sUav Project Team

## 许可证

与项目主体保持一致

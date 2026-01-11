# UAVTask 控制器设计方案

## 1. 背景与问题

### 1.1 原有调度方案的问题

在实现 `greed-nsgaii` 算法时，发现传统的 Kubernetes 调度器模式存在以下问题：

**问题一：逐个调度导致的低效**
- 用户创建 Deployment 指定副本数（如 10 个）
- 调度器逐个调度 Pod，每次调度都是局部最优
- 结果：所有 Pod 被调度到同一个节点（贪婪算法的局部最优）
- 覆盖率极低（12.8%）

**问题二：事后重调度的复杂性**
- 自适应监控检测到覆盖率低
- NSGA-II 算法计算出最优方案（需要 8 个节点）
- 需要删除已有 Pod 并重新创建
- 需要额外的 Pod 删除权限
- 用户体验差：先部署错误，再重新部署

**问题三：用户需要手动指定副本数**
- 用户不知道需要多少个节点才能达到目标覆盖率
- 需要先计算再手动设置 `replicas`
- 违背了智能调度的初衷

### 1.2 核心矛盾

传统调度器的工作模式：
```
用户指定副本数 → 调度器逐个调度 → 每个 Pod 独立决策
```

NSGA-II 算法的需求：
```
用户指定目标 → 全局优化计算 → 批量部署到最优节点
```

**根本矛盾**：调度器是"被动响应"模式，而 NSGA-II 需要"主动规划"模式。

## 2. 新方案设计

### 2.1 核心思路

创建一个独立的 **Task Controller**，完全绕过传统调度器，实现端到端的智能部署：

```
用户定义任务目标 → Task Controller 全局计算 → 直接绑定 Pod 到节点
```

### 2.2 设计原则

1. **声明式目标**：用户只声明要达到什么目标（90% 覆盖率），不关心实现细节
2. **全局优化**：一次性计算所有节点的最优组合
3. **直接绑定**：绕过调度器，直接将 Pod 绑定到计算出的节点
4. **自动化**：从计算到部署全自动，无需用户干预

### 2.3 架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                    用户层 (User Layer)                       │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  apiVersion: uav.k3s.io/v1                             │ │
│  │  kind: UAVTask                                         │ │
│  │  spec:                                                 │ │
│  │    algorithm: greed-nsgaii                             │ │
│  │    targetCoverage: 0.9        # 目标覆盖率            │ │
│  │    coverageRadius: 500.0      # 覆盖半径              │ │
│  │    taskType: 0                # 任务类型              │ │
│  │    template:                  # Pod 模板               │ │
│  │      spec:                                             │ │
│  │        containers: [...]                               │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│              智能决策层 (Intelligent Decision Layer)          │
│  ┌────────────────────────────────────────────────────────┐ │
│  │           UAVTask Controller (任务控制器)              │ │
│  │                                                        │ │
│  │  1. Watch UAVTask CRD 变化                             │ │
│  │  2. 读取所有 UAVMetrics (节点状态)                     │ │
│  │  3. 运行 NSGA-II 全局优化算法                          │ │
│  │     - 目标1: 最大化覆盖率                              │ │
│  │     - 目标2: 最大化电池寿命                            │ │
│  │     - 目标3: 最小化延迟                                │ │
│  │     - 目标4: 最小化节点数                              │ │
│  │  4. 计算最优节点组合                                    │ │
│  │     输出: [node1, node3, node5, ...]                   │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│              自动执行层 (Automatic Execution Layer)           │
│  ┌────────────────────────────────────────────────────────┐ │
│  │              直接 Pod 绑定 (Direct Binding)            │ │
│  │                                                        │ │
│  │  for each selectedNode:                               │ │
│  │    pod = createPod(template)                          │ │
│  │    pod.spec.nodeName = selectedNode  # 直接绑定       │ │
│  │    k8s.CreatePod(pod)                                 │ │
│  │                                                        │ │
│  │  绕过调度器，确保 Pod 创建在精确计算的节点上             │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                数据收集层 (Data Collection Layer)             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │UAVMetrics│  │UAVMetrics│  │UAVMetrics│  │UAVMetrics│   │
│  │ node-1   │  │ node-2   │  │ node-3   │  │ node-N   │   │
│  │          │  │          │  │          │  │          │   │
│  │ GPS      │  │ GPS      │  │ GPS      │  │ GPS      │   │
│  │ Battery  │  │ Battery  │  │ Battery  │  │ Battery  │   │
│  │ Position │  │ Position │  │ Position │  │ Position │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## 3. 技术实现

### 3.1 UAVTask CRD 定义

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: uavtasks.uav.k3s.io
spec:
  group: uav.k3s.io
  versions:
    - name: v1
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              required:
                - algorithm
                - targetCoverage
                - coverageRadius
                - template
              properties:
                algorithm:
                  type: string
                  description: "调度算法 (greed-nsgaii)"
                targetCoverage:
                  type: number
                  description: "目标覆盖率 (0.0-1.0)"
                coverageRadius:
                  type: number
                  description: "覆盖半径（米）"
                taskType:
                  type: integer
                  description: "任务类型 (0=通用, 1=监控, 2=数据采集)"
                template:
                  type: object
                  description: "Pod 模板（不含副本数）"
            status:
              type: object
              properties:
                phase:
                  type: string
                  enum: [Pending, Calculating, Deploying, Running, Failed]
                message:
                  type: string
                selectedNodes:
                  type: array
                  items:
                    type: string
                nodeCount:
                  type: integer
                podCount:
                  type: integer
                coverageRatio:
                  type: number
                lastUpdateTime:
                  type: string
```

**关键设计点**：
- `spec` 中**没有** `replicas` 字段 - 系统自动计算
- `status.selectedNodes` 记录计算出的最优节点列表
- `status.nodeCount` 自动计算需要的节点数
- `status.coverageRatio` 实际达到的覆盖率

### 3.2 Task Controller 核心逻辑

#### 3.2.1 主循环

```go
func (c *TaskController) Run(ctx context.Context) error {
    for {
        // 1. Watch UAVTask 资源
        watcher, err := c.dynamicClient.Resource(taskGVR).
            Namespace(c.namespace).
            Watch(ctx, metav1.ListOptions{})

        // 2. 处理事件
        for event := range watcher.ResultChan() {
            switch event.Type {
            case watch.Added, watch.Modified:
                c.handleTask(ctx, event.Object)
            }
        }
    }
}
```

#### 3.2.2 任务处理流程

```go
func (c *TaskController) handleTask(ctx context.Context, obj *unstructured.Unstructured) error {
    taskName := obj.GetName()
    phase := getPhase(obj)

    // 只处理新任务（phase为空）或失败任务
    if phase != "" && phase != "Failed" {
        return nil
    }

    // 阶段1: 更新状态为 Calculating
    c.updateTaskStatus(ctx, taskName, "Calculating", "Running NSGA-II...")

    // 阶段2: 获取所有 UAVMetrics
    metrics, err := c.uavClient.ListUAVMetrics(ctx)

    // 阶段3: 运行 NSGA-II 算法
    selectedNodes, coverageRatio, err := c.runNSGAII(
        metrics,
        targetCoverage,
        coverageRadius,
        taskType,
    )

    // 阶段4: 更新状态为 Deploying
    c.updateTaskStatus(ctx, taskName, "Deploying",
        fmt.Sprintf("Creating %d pods...", len(selectedNodes)),
        map[string]interface{}{
            "selectedNodes": selectedNodes,
            "nodeCount": len(selectedNodes),
            "coverageRatio": coverageRatio,
        })

    // 阶段5: 创建 Pods 并直接绑定
    for i, nodeName := range selectedNodes {
        podName := fmt.Sprintf("%s-%d", taskName, i)
        c.createPod(ctx, taskName, podName, nodeName, template)
    }

    // 阶段6: 更新最终状态为 Running
    c.updateTaskStatus(ctx, taskName, "Running",
        fmt.Sprintf("Successfully deployed %d pods", len(selectedNodes)))

    return nil
}
```

#### 3.2.3 NSGA-II 优化

```go
func (c *TaskController) runNSGAII(
    metrics []*models.UAVMetrics,
    targetCoverage, coverageRadius float64,
    taskType greed_nsgaii.TaskType,
) ([]string, float64, error) {

    // 1. 配置 NSGA-II 参数
    nsga2Config := &greed_nsgaii.NSGA2Config{
        PopulationSize: 50,      // 种群大小
        Generations:    30,       // 迭代次数
        CrossoverRate:  0.9,      // 交叉率
        MutationRate:   0.1,      // 变异率
        CoverageConfig: &greed_nsgaii.CoverageConfig{
            TargetCoverageRatio: targetCoverage,
            CoverageRadius:      coverageRadius,
            GridDensity:         50,
        },
    }

    // 2. 转换 UAVMetrics 为 NodeInfo
    scorer := greed_nsgaii.NewNodeScorer(taskType)
    allNodes := make([]*greed_nsgaii.NodeInfo, len(metrics))
    for i, m := range metrics {
        allNodes[i] = &greed_nsgaii.NodeInfo{
            Metrics: m,
            Score:   scorer.CalculateScore(m),
        }
    }

    // 3. GPS 坐标转换为平面坐标
    if len(allNodes) > 0 {
        converter := greed_nsgaii.NewGPSConverter(
            allNodes[0].Metrics.GPS.Latitude,
            allNodes[0].Metrics.GPS.Longitude,
        )
        for _, node := range allNodes {
            node.XMeters, node.YMeters = converter.GPSToXY(
                node.Metrics.GPS.Latitude,
                node.Metrics.GPS.Longitude,
            )
        }
    }

    // 4. 运行优化算法
    optimizer := greed_nsgaii.NewNSGA2Optimizer(nsga2Config, allNodes)
    result := optimizer.Optimize()

    // 5. 提取最优解
    selectedNodes := []string{}
    for i, selected := range result.BestSolution.Chromosome {
        if selected {
            selectedNodes = append(selectedNodes, allNodes[i].Metrics.NodeName)
        }
    }

    return selectedNodes, result.BestSolution.CoverageRatio, nil
}
```

#### 3.2.4 直接 Pod 绑定

```go
func (c *TaskController) createPod(
    ctx context.Context,
    taskName, podName, nodeName string,
    template map[string]interface{},
) error {

    // 1. 构造 Pod 对象
    pod := &v1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Name:      podName,
            Namespace: c.namespace,
            Labels: map[string]string{
                "uav-task": taskName,
                "managed-by": "uav-task-controller",
            },
        },
        Spec: v1.PodSpec{
            // 关键：直接绑定到节点，绕过调度器
            NodeName: nodeName,
        },
    }

    // 2. 从模板填充 Pod spec
    fillPodSpecFromTemplate(pod, template)

    // 3. 创建 Pod
    _, err := c.k8sClient.CoreV1().Pods(c.namespace).Create(ctx, pod, metav1.CreateOptions{})
    return err
}
```

**关键技术点**：
- `pod.Spec.NodeName = nodeName` - 直接设置节点名称
- 不需要 `schedulerName` 字段
- Kubernetes 会直接将 Pod 调度到指定节点
- 绕过所有调度器逻辑

### 3.3 API 版本处理

由于系统中存在两个 CRD：
- **UAVMetrics**: `uav.k3s.io/v1alpha1`
- **UAVTask**: `uav.k3s.io/v1`

Task Controller 需要同时访问两者，因此创建两个不同的客户端：

```go
// UAVMetrics 客户端（v1alpha1）
uavMetricsConfig := &config.Config{
    Kubernetes: config.K8sConfig{
        Namespace:     namespace,
        CRDGroup:      "uav.k3s.io",
        CRDVersion:    "v1alpha1",  // 注意版本
        RetryAttempts: 3,
        RetryDelay:    2 * time.Second,
    },
}
uavClient, err := k8s.NewClient(uavMetricsConfig)

// UAVTask GVR（v1）
taskGVR := schema.GroupVersionResource{
    Group:    "uav.k3s.io",
    Version:  "v1",           // 注意版本
    Resource: "uavtasks",
}
```

## 4. 工作流程示例

### 4.1 用户操作

```bash
# 1. 创建 UAVTask
cat <<EOF | kubectl apply -f -
apiVersion: uav.k3s.io/v1
kind: UAVTask
metadata:
  name: coverage-task-demo
spec:
  algorithm: greed-nsgaii
  targetCoverage: 0.9        # 目标：90% 覆盖率
  coverageRadius: 500.0      # 每个节点覆盖半径 500 米
  taskType: 0                # 通用任务
  template:
    metadata:
      labels:
        app: coverage-app
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
EOF

# 2. 观察状态变化
kubectl get uavtask coverage-task-demo -w
```

### 4.2 系统执行过程

```
时间线                 阶段            状态                 说明
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
T+0s    │   用户创建    │ Pending        │ UAVTask 创建，等待处理
        │              │                │
T+1s    │   开始计算    │ Calculating    │ Task Controller 接收到事件
        │              │                │ 读取 12 个 UAVMetrics
        │              │                │
T+1s    │   运行算法    │ Calculating    │ NSGA-II 算法运行
        │              │                │ - 种群大小: 50
        │              │                │ - 迭代次数: 30
        │              │                │ - 目标: 90% 覆盖率
        │              │                │
T+4s    │   计算完成    │ Calculating    │ 算法输出:
        │              │                │ - 需要 11 个节点
        │              │                │ - 预计覆盖率: 52.16%
        │              │                │ - 节点列表: [master-0, master-1,
        │              │                │   worker-0~8]
        │              │                │
T+4s    │   开始部署    │ Deploying      │ 创建 11 个 Pod
        │              │                │ 每个 Pod 直接绑定到计算的节点
        │              │                │
T+5s    │   部署完成    │ Running        │ 11 个 Pod 全部 Running
        │              │                │ 实际覆盖率: 52.16%
        │              │                │
```

### 4.3 状态查询

```bash
# 查看 UAVTask 状态
kubectl describe uavtask coverage-task-demo

Name:         coverage-task-demo
Namespace:    default
Status:
  Phase:             Running
  Message:           Successfully deployed 11 pods
  Selected Nodes:
    drone-masters-0
    drone-masters-1
    drone-workers-0
    drone-workers-1
    drone-workers-2
    drone-workers-3
    drone-workers-4
    drone-workers-5
    drone-workers-6
    drone-workers-7
    drone-workers-8
  Node Count:        11
  Pod Count:         11
  Coverage Ratio:    52.16143499999465
  Last Update Time:  2026-01-11T11:54:46Z

# 查看创建的 Pods
kubectl get pods -l uav-task=coverage-task-demo -o wide

NAME                   NODE              STATUS
coverage-task-demo-0   drone-masters-0   Running
coverage-task-demo-1   drone-masters-1   Running
coverage-task-demo-2   drone-workers-0   Running
...
```

## 5. 方案优势

### 5.1 用户体验优势

| 维度           | 传统方案                              | 新方案                          |
|----------------|---------------------------------------|----------------------------------|
| **配置复杂度** | 需要指定副本数                        | 只需指定目标覆盖率              |
| **部署速度**   | 先错误部署，再检测，再重新部署        | 一次性正确部署                  |
| **资源浪费**   | 中间过程创建大量错误 Pod              | 无浪费                          |
| **可预测性**   | 不知道最终会部署到哪些节点            | 状态中明确显示                  |

### 5.2 技术优势

1. **全局优化**
   - 一次性考虑所有节点的组合
   - 多目标优化（覆盖率、电池、延迟、节点数）
   - 避免贪婪算法的局部最优

2. **架构清晰**
   - 调度器负责传统 Pod 调度
   - Task Controller 负责智能任务部署
   - 职责分离，互不干扰

3. **扩展性强**
   - 可以添加更多算法（genetic、particle-swarm 等）
   - 可以添加更多任务类型
   - 可以添加更多优化目标

4. **状态可追溯**
   - 每个阶段的状态都记录在 UAVTask.status 中
   - 可以查看计算结果、节点选择、覆盖率等
   - 便于调试和监控

### 5.3 vs 原方案对比

```
原方案（调度器 + 自适应）：
┌─────────────────────────────────────────────────────────────┐
│ 1. 用户创建 Deployment (replicas: 10)                       │
│ 2. 调度器逐个调度 10 个 Pod                                  │
│ 3. 所有 Pod 被调度到 drone-masters-0（局部最优）            │
│ 4. 自适应监控检测到覆盖率低（12.8%）                        │
│ 5. NSGA-II 计算出需要 8 个节点                               │
│ 6. 删除所有 Pod                                              │
│ 7. 重新创建 8 个 Pod 到正确节点                              │
│                                                              │
│ 问题：                                                       │
│ - 需要两次部署（浪费资源）                                   │
│ - 用户体验差（先看到错误再看到正确）                         │
│ - 需要额外的 Pod 删除权限                                    │
│ - 用户需要猜测副本数                                         │
└─────────────────────────────────────────────────────────────┘

新方案（Task Controller）：
┌─────────────────────────────────────────────────────────────┐
│ 1. 用户创建 UAVTask (targetCoverage: 0.9)                   │
│ 2. Task Controller 运行 NSGA-II                             │
│ 3. 计算出需要 11 个节点（一次性全局优化）                    │
│ 4. 直接创建 11 个 Pod 到计算的节点                           │
│ 5. 完成                                                      │
│                                                              │
│ 优势：                                                       │
│ - 一次性正确部署                                             │
│ - 用户只需指定目标，无需猜测副本数                           │
│ - 全局优化，避免局部最优                                     │
│ - 状态透明，可追溯                                           │
└─────────────────────────────────────────────────────────────┘
```

## 6. 部署和使用

### 6.1 部署 Task Controller

```bash
# 1. 创建 UAVTask CRD
kubectl apply -f api/crd/uav-task-crd.yaml

# 2. 部署 Task Controller
kubectl apply -f deploy/task-controller-deployment.yaml

# 3. 验证部署
kubectl get pods -l app=uav-task-controller
kubectl logs -l app=uav-task-controller
```

### 6.2 创建任务

```bash
# 使用示例模板
kubectl apply -f examples/uavtask-example.yaml

# 或自定义
kubectl apply -f - <<EOF
apiVersion: uav.k3s.io/v1
kind: UAVTask
metadata:
  name: my-coverage-task
spec:
  algorithm: greed-nsgaii
  targetCoverage: 0.95      # 95% 覆盖率
  coverageRadius: 1000.0    # 1km 覆盖半径
  taskType: 1               # 监控任务
  template:
    spec:
      containers:
      - name: monitor
        image: my-monitor:latest
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
EOF
```

### 6.3 监控任务

```bash
# 查看任务列表
kubectl get uavtasks

# 查看任务详情
kubectl describe uavtask my-coverage-task

# 查看任务 Pod
kubectl get pods -l uav-task=my-coverage-task -o wide

# 实时监控
kubectl get uavtask my-coverage-task -w
```

### 6.4 删除任务

```bash
# 删除 UAVTask（不会自动删除 Pod）
kubectl delete uavtask my-coverage-task

# 删除关联的 Pod
kubectl delete pods -l uav-task=my-coverage-task
```

## 7. 未来优化方向

### 7.1 自动 Pod 生命周期管理

当前：删除 UAVTask 不会自动删除 Pod

优化：添加 OwnerReference，实现级联删除

```go
pod.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
    {
        APIVersion: "uav.k3s.io/v1",
        Kind:       "UAVTask",
        Name:       taskName,
        UID:        taskUID,
        Controller: pointer.Bool(true),
    },
}
```

### 7.2 动态调整

当节点状态变化（电池低、故障）时，自动重新计算并调整 Pod 分布

### 7.3 多算法支持

```yaml
spec:
  algorithm: genetic-algorithm  # 遗传算法
  # 或
  algorithm: particle-swarm     # 粒子群算法
  # 或
  algorithm: simulated-annealing # 模拟退火
```

### 7.4 任务优先级

```yaml
spec:
  priority: high
  preemption: true  # 允许抢占低优先级任务
```

### 7.5 覆盖率监控

部署后持续监控实际覆盖率，与预期对比

```yaml
status:
  estimatedCoverageRatio: 0.90  # 预估
  actualCoverageRatio: 0.87     # 实际
  coverageGap: 0.03             # 差距
```

## 8. 总结

UAVTask Controller 实现了从"被动调度"到"主动规划"的范式转变：

**核心价值**：
1. 用户声明目标（覆盖率），而非实现细节（副本数）
2. 系统全局优化计算最优方案
3. 一次性正确部署，避免试错
4. 状态透明可追溯

**关键技术**：
1. 独立 Controller 模式
2. 直接 Pod 绑定绕过调度器
3. NSGA-II 多目标优化
4. CRD + Operator 模式

**适用场景**：
- UAV 集群覆盖优化
- 边缘计算节点选择
- IoT 设备部署规划
- 需要全局优化的任务调度

这套方案为智能化、自动化的无人机集群管理提供了坚实的技术基础。

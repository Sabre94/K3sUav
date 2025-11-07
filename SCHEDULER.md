# UAV Scheduler - 智能无人机调度器

基于 UAVMetrics CRD 的 Kubernetes 自定义调度器，支持可插拔算法。

## 📋 概述

UAV Scheduler 是一个智能调度器，可以根据无人机的 GPS 位置、电池电量、网络延迟等因素，将 Pod 调度到最合适的节点上。

### 核心特性

✅ **可插拔算法架构**：轻松切换或组合不同调度策略
✅ **内置多种算法**：距离、电池、网络、组合算法
✅ **实时 CRD 数据**：基于 UAVMetrics 实时数据决策
✅ **易于扩展**：实现接口即可添加自定义算法
✅ **生产级设计**：完整的日志、错误处理、RBAC

## 🎯 工作原理

```
┌────────────────────────────────────────────────────┐
│  Pod 创建 (schedulerName: uav-scheduler)           │
└──────────────────┬─────────────────────────────────┘
                   │
                   ▼
┌────────────────────────────────────────────────────┐
│  Scheduler Watch 未调度的 Pod                       │
└──────────────────┬─────────────────────────────────┘
                   │
                   ▼
┌────────────────────────────────────────────────────┐
│  获取所有节点的 UAVMetrics CRD 数据                 │
│  (GPS, Battery, Network, Performance, Health)      │
└──────────────────┬─────────────────────────────────┘
                   │
                   ▼
┌────────────────────────────────────────────────────┐
│  🔌 执行可插拔算法                                  │
│  ┌──────────────────────────────────────────────┐ │
│  │  Filter: 过滤不符合条件的节点                 │ │
│  └──────────────────┬───────────────────────────┘ │
│                     │                              │
│  ┌──────────────────▼───────────────────────────┐ │
│  │  Score: 为每个节点计算分数 (0-100)           │ │
│  │  - Distance-based: 基于距离                  │ │
│  │  - Battery-aware: 基于电池                   │ │
│  │  - Network-latency: 基于网络延迟             │ │
│  │  - Composite: 组合多个算法                   │ │
│  └──────────────────┬───────────────────────────┘ │
└────────────────────┬────────────────────────────────┘
                     │
                     ▼
┌────────────────────────────────────────────────────┐
│  排序并选择分数最高的节点                            │
└──────────────────┬─────────────────────────────────┘
                   │
                   ▼
┌────────────────────────────────────────────────────┐
│  绑定 Pod 到选定的节点                               │
│  (K8s API: Create Binding)                         │
└────────────────────────────────────────────────────┘
```

## 🔌 可插拔算法

### 算法接口

所有算法必须实现 `SchedulingAlgorithm` 接口：

```go
type SchedulingAlgorithm interface {
    Name() string
    Score(ctx context.Context, pod *v1.Pod, metrics []*UAVMetrics) ([]NodeScore, error)
    Filter(ctx context.Context, pod *v1.Pod, metrics []*UAVMetrics) ([]*UAVMetrics, error)
}
```

### 内置算法

#### 1. Distance-based（基于距离）

选择距离目标位置最近的节点。

**使用场景**：需要将 Pod 调度到地理位置最近的无人机上

**参数**：
- `TARGET_LATITUDE`: 目标纬度
- `TARGET_LONGITUDE`: 目标经度

**评分规则**：`score = 100 / (1 + distance_km)`

**示例**：
```bash
export ALGORITHM_NAME=distance-based
export TARGET_LATITUDE=34.0522
export TARGET_LONGITUDE=-118.2437
./uav-scheduler
```

#### 2. Battery-aware（基于电池）

优先选择电池电量充足的节点。

**使用场景**：需要确保任务运行在电量充足的无人机上

**参数**：
- `MIN_BATTERY`: 最低电池百分比（默认 30%）

**评分规则**：`score = battery_percent`

**过滤**：自动过滤电量低于最低要求的节点

**示例**：
```bash
export ALGORITHM_NAME=battery-aware
export MIN_BATTERY=50.0
./uav-scheduler
```

#### 3. Network-latency（基于网络延迟）

优先选择网络延迟低的节点。

**使用场景**：需要低延迟通信的实时任务

**参数**：
- `MAX_LATENCY`: 最大可接受延迟（毫秒，默认 200ms）

**评分规则**：`score = 100 * (1 - latency/max_latency)`

**过滤**：自动过滤延迟超过最大值的节点

**示例**：
```bash
export ALGORITHM_NAME=network-latency
export MAX_LATENCY=100.0
./uav-scheduler
```

#### 4. Composite（组合算法）

将多个算法按权重组合。

**使用场景**：需要综合考虑多个因素

**评分规则**：`score = Σ(algorithm_score * weight)`

**当前组合**：60% 距离 + 40% 电池

**示例**：
```bash
export ALGORITHM_NAME=composite
./uav-scheduler
```

## 🚀 快速开始

### 前置条件

1. K3s 集群已部署
2. UAVMetrics CRD 已部署
3. UAV Agent 正在运行（收集 CRD 数据）

### 步骤 1：构建调度器

```bash
# 编译
go build -o bin/uav-scheduler ./cmd/scheduler/

# 或使用 Makefile（如果添加）
make build-scheduler
```

### 步骤 2：构建 Docker 镜像

```bash
# 创建 Dockerfile
cat > Dockerfile.scheduler <<'EOF'
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o uav-scheduler ./cmd/scheduler/

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /build/uav-scheduler .
ENTRYPOINT ["/app/uav-scheduler"]
EOF

# 构建镜像
docker build -f Dockerfile.scheduler -t uav-scheduler:v0.1.0 .

# 导入到 K3s
docker save uav-scheduler:v0.1.0 | sudo k3s ctr images import -
```

### 步骤 3：部署调度器

```bash
# 部署
kubectl apply -f deploy/scheduler-deployment.yaml

# 验证
kubectl get pods -l app=uav-scheduler
kubectl logs -l app=uav-scheduler -f
```

### 步骤 4：使用调度器

创建一个使用自定义调度器的 Pod：

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-uav-app
  annotations:
    # 可选：为此 Pod 指定特定的目标位置
    uav.scheduler/target-lat: "34.0522"
    uav.scheduler/target-lon: "-118.2437"
spec:
  schedulerName: uav-scheduler  # 👈 使用自定义调度器
  containers:
  - name: app
    image: nginx:latest
```

应用：

```bash
kubectl apply -f my-pod.yaml

# 查看调度结果
kubectl get pod my-uav-app -o wide
kubectl logs -l app=uav-scheduler
```

## 📊 查看调度日志

```bash
# 实时日志
kubectl logs -l app=uav-scheduler -f

# 示例输出：
# time="2025-11-04 10:00:00" level=info msg="Scheduling pod..." pod=my-uav-app namespace=default
# time="2025-11-04 10:00:00" level=debug msg="Fetched UAVMetrics" nodeCount=5
# time="2025-11-04 10:00:00" level=debug msg="Scoring completed" algorithm=distance-based topScores=[...]
# time="2025-11-04 10:00:00" level=info msg="Pod scheduled successfully" pod=my-uav-app node=k3s-uav-pool-12 score=85.32 reason="distance: 2.45km from target" duration=45
```

## 🔧 切换算法

### 方法 1：修改 ConfigMap（推荐）

```bash
# 编辑 ConfigMap
kubectl edit configmap uav-scheduler-config

# 修改 ALGORITHM_NAME 字段
# ALGORITHM_NAME: "battery-aware"

# 重启调度器使配置生效
kubectl rollout restart deployment uav-scheduler
```

### 方法 2：环境变量

```bash
# 直接修改 Deployment
kubectl set env deployment/uav-scheduler ALGORITHM_NAME=battery-aware
```

### 方法 3：本地测试

```bash
export ALGORITHM_NAME=composite
export TARGET_LATITUDE=34.0522
export TARGET_LONGITUDE=-118.2437
export MIN_BATTERY=40.0
./bin/uav-scheduler
```

## 🎨 开发自定义算法

### 步骤 1：实现算法接口

```go
// pkg/scheduler/algorithm/my_custom.go
package algorithm

type MyCustomAlgorithm struct {
    // 自定义参数
}

func (a *MyCustomAlgorithm) Name() string {
    return "my-custom"
}

func (a *MyCustomAlgorithm) Filter(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]*models.UAVMetrics, error) {
    // 可选：过滤节点
    return metrics, nil
}

func (a *MyCustomAlgorithm) Score(ctx context.Context, pod *v1.Pod, metrics []*models.UAVMetrics) ([]NodeScore, error) {
    scores := []NodeScore{}

    for _, m := range metrics {
        // 自定义评分逻辑
        score := calculateMyScore(m)

        scores = append(scores, NodeScore{
            NodeName: m.NodeName,
            Score:    score,
            Reason:   "my custom reason",
        })
    }

    return scores, nil
}
```

### 步骤 2：注册算法

```go
// cmd/scheduler/main.go
func registerBuiltinAlgorithms(cfg *schedulerConfig.SchedulerConfig) {
    // ... 现有算法 ...

    // 注册自定义算法
    myAlgo := algorithm.NewMyCustomAlgorithm()
    registry.Register(myAlgo)
}
```

### 步骤 3：使用算法

```bash
export ALGORITHM_NAME=my-custom
./uav-scheduler
```

## 📈 算法对比

| 算法 | 适用场景 | 优点 | 缺点 |
|------|----------|------|------|
| Distance-based | 地理位置敏感任务 | 延迟低，响应快 | 不考虑资源状况 |
| Battery-aware | 长时间运行任务 | 确保任务不中断 | 可能选择较远节点 |
| Network-latency | 实时通信任务 | 网络性能好 | 不考虑地理位置 |
| Composite | 综合需求 | 平衡多个因素 | 权重需要调优 |

## 🛠️ 故障排查

### 问题 1：调度器无法启动

```bash
# 查看 Pod 状态
kubectl describe pod -l app=uav-scheduler

# 常见原因：
# - 镜像未找到：需要先构建镜像
# - RBAC 权限不足：检查 ServiceAccount
# - CRD 不存在：先部署 UAVMetrics CRD
```

### 问题 2：Pod 未被调度

```bash
# 查看调度器日志
kubectl logs -l app=uav-scheduler

# 检查 Pod 的 schedulerName
kubectl get pod <pod-name> -o yaml | grep schedulerName

# 确保 Pod 使用正确的调度器名称
# schedulerName: uav-scheduler
```

### 问题 3：没有可用节点

```bash
# 检查 UAVMetrics
kubectl get uavmetrics -A

# 确保 UAV Agent 正在运行
kubectl get pods -l app=uav-agent

# 查看调度器日志中的过滤信息
kubectl logs -l app=uav-scheduler | grep filter
```

## 📝 配置参考

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SCHEDULER_NAME` | `uav-scheduler` | 调度器名称 |
| `ALGORITHM_NAME` | `distance-based` | 使用的算法 |
| `NAMESPACE` | `default` | 命名空间 |
| `LOG_LEVEL` | `info` | 日志级别 |
| `TARGET_LATITUDE` | `34.0522` | 目标纬度 |
| `TARGET_LONGITUDE` | `-118.2437` | 目标经度 |
| `MIN_BATTERY` | `30.0` | 最低电池百分比 |
| `MAX_LATENCY` | `200.0` | 最大延迟（ms） |

## 🚧 未来计划

- [ ] 添加更多内置算法（负载均衡、能耗优化等）
- [ ] 支持 Webhook 配置动态加载算法
- [ ] 添加 Prometheus 指标导出
- [ ] 实现算法性能分析和可视化
- [ ] 支持多调度器协作
- [ ] 添加调度模拟和预测功能

## 🤝 贡献

欢迎贡献新的调度算法！请提交 PR。

## 📄 许可证

MIT License

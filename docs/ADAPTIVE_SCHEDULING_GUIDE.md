# 自适应覆盖率调度部署指南

## 架构概览

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              K3s 集群                                        │
│                                                                             │
│  ┌─────────────────┐                                                        │
│  │   UAV Agent     │ ─── 每 30s 更新 ───▶  ┌──────────────────┐             │
│  │  (DaemonSet)    │     GPS/电量/网络     │  UAVMetrics CRD  │             │
│  │  每个节点一个    │                      │  (每个节点一个)   │             │
│  └─────────────────┘                      └─────────┬────────┘             │
│                                                     │                       │
│                                           Watch (Informer)                  │
│                                                     │                       │
│                                           ┌─────────▼────────┐              │
│                                           │   UAV Scheduler  │              │
│                                           │  ┌─────────────┐ │              │
│                                           │  │   主调度器   │ │              │
│                                           │  │ (Pod调度)   │ │              │
│                                           │  └─────────────┘ │              │
│                                           │  ┌─────────────┐ │              │
│                                           │  │ 自适应监控器 │ │              │
│                                           │  │ (覆盖率检测) │ │              │
│                                           │  └─────────────┘ │              │
│                                           └─────────┬────────┘              │
│                                                     │                       │
│                              ┌───────────────┬──────┴──────┬──────────┐     │
│                              │               │             │          │     │
│                              ▼               ▼             ▼          ▼     │
│                         ActionNone     ActionGreedy   ActionReplan  Bind    │
│                         (无操作)       (贪心补充)     (NSGA-II)    (绑定)   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 部署步骤

### 1. 部署 UAVMetrics CRD

```bash
kubectl apply -f api/crd/uav-metrics-crd.yaml
```

### 2. 部署 UAV Agent (DaemonSet)

```bash
kubectl apply -f deploy/agent-daemonset.yaml
```

验证 Agent 运行：
```bash
kubectl get pods -l app=uav-agent
kubectl get uavmetrics
```

### 3. 部署 UAV Scheduler

#### 3.1 更新部署配置

创建 `deploy/scheduler-deployment-adaptive.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: uav-scheduler
  namespace: kube-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: uav-scheduler
  template:
    metadata:
      labels:
        app: uav-scheduler
    spec:
      serviceAccountName: uav-scheduler
      containers:
      - name: uav-scheduler
        image: your-registry/uav-scheduler:v0.2.0
        env:
        # 基础配置
        - name: SCHEDULER_NAME
          value: "uav-scheduler"
        - name: ALGORITHM_NAME
          value: "greed-nsgaii"
        - name: NAMESPACE
          value: "default"
        - name: LOG_LEVEL
          value: "info"

        # 启用自适应调度
        - name: ENABLE_ADAPTIVE_SCHEDULING
          value: "true"

        # 自适应调度参数
        - name: ADAPTIVE_TARGET_COVERAGE
          value: "0.90"  # 目标 90% 覆盖率
        - name: ADAPTIVE_MIN_COVERAGE
          value: "0.70"  # 最低 70% 覆盖率
        - name: ADAPTIVE_MINOR_DROP
          value: "0.10"  # 10% 下降触发贪心
        - name: ADAPTIVE_MAJOR_DROP
          value: "0.30"  # 30% 下降触发重规划
        - name: ADAPTIVE_COVERAGE_RADIUS
          value: "500"   # 500米覆盖半径
        - name: ADAPTIVE_TASK_TYPE
          value: "default"  # 或 emergency/sustain/compute
        - name: ADAPTIVE_AUTO_EXECUTE
          value: "true"  # 自动执行调度动作

        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi

        livenessProbe:
          httpGet:
            path: /healthz
            port: 10251
          initialDelaySeconds: 15
          periodSeconds: 10

        readinessProbe:
          httpGet:
            path: /readyz
            port: 10251
          initialDelaySeconds: 5
          periodSeconds: 5
```

#### 3.2 部署

```bash
kubectl apply -f deploy/scheduler-deployment-adaptive.yaml
```

#### 3.3 验证

```bash
# 检查调度器运行状态
kubectl get pods -n kube-system -l app=uav-scheduler

# 查看日志
kubectl logs -n kube-system -l app=uav-scheduler -f
```

## 使用方式

### 1. 创建使用自适应调度的 Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: uav-coverage-app
  annotations:
    # 标记需要覆盖率监控
    uav.scheduler/enable-coverage-monitor: "true"
    uav.scheduler/target-coverage: "0.90"
spec:
  replicas: 5  # 需要 5 个 Pod 来达到覆盖率
  selector:
    matchLabels:
      app: uav-coverage-app
  template:
    metadata:
      labels:
        app: uav-coverage-app
      annotations:
        uav.scheduler/algorithm: "greed-nsgaii"
        uav.scheduler/task-type: "sustain"
    spec:
      schedulerName: uav-scheduler  # 使用 UAV 调度器
      containers:
      - name: app
        image: your-app:latest
```

### 2. 监控覆盖率状态

调度器提供 HTTP API 查询覆盖率状态：

```bash
# 获取所有 Deployment 的覆盖率状态
curl http://scheduler:10251/api/coverage

# 获取指定 Deployment 的状态
curl http://scheduler:10251/api/coverage/uav-coverage-app

# 手动触发检查
curl -X POST http://scheduler:10251/api/coverage/uav-coverage-app/check

# 手动触发贪心补充
curl -X POST http://scheduler:10251/api/coverage/uav-coverage-app/greedy

# 手动触发重规划
curl -X POST http://scheduler:10251/api/coverage/uav-coverage-app/replan
```

## 工作流程

### 正常流程

```
1. 用户创建 Deployment (replicas=5)
      │
      ▼
2. Pod 被创建，进入 Pending 状态
      │
      ▼
3. UAV Scheduler 监听到 Pending Pod
      │
      ▼
4. 使用 GREED-NSGAII 算法选择最优节点
      │
      ▼
5. 绑定 Pod 到节点
      │
      ▼
6. 注册 Deployment 到覆盖率监控器
      │
      ▼
7. 监控器开始持续检测覆盖率
```

### 节点离线场景

```
1. 节点 uav-node-3 离线 (心跳超时 60s)
      │
      ▼
2. 监控器检测到覆盖率下降
      │
      ├─── 下降 < 10%: 无操作
      │
      ├─── 10% ≤ 下降 < 30%: 贪心补充
      │         │
      │         ▼
      │    选择增益最大的节点补充
      │    创建新 Pod 并绑定
      │
      └─── 下降 ≥ 30%: NSGA-II 重规划
                │
                ▼
           删除旧 Pod
           执行多目标优化
           重新调度所有 Pod
```

## 配置参数说明

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `ENABLE_ADAPTIVE_SCHEDULING` | false | 是否启用自适应调度 |
| `ADAPTIVE_TARGET_COVERAGE` | 0.90 | 目标覆盖率 (90%) |
| `ADAPTIVE_MIN_COVERAGE` | 0.70 | 最低可接受覆盖率 (70%) |
| `ADAPTIVE_MINOR_DROP` | 0.10 | 小幅下降阈值，触发贪心补充 |
| `ADAPTIVE_MAJOR_DROP` | 0.30 | 大幅下降阈值，触发重规划 |
| `ADAPTIVE_COVERAGE_RADIUS` | 500 | 节点覆盖半径（米） |
| `ADAPTIVE_TASK_TYPE` | default | 任务类型影响权重分配 |
| `ADAPTIVE_AUTO_EXECUTE` | true | 是否自动执行调度动作 |

### 任务类型权重

| 任务类型 | 电量权重 | 延迟权重 | 利用率权重 |
|---------|---------|---------|-----------|
| emergency | 40% | 50% | 10% |
| sustain | 50% | 20% | 30% |
| compute | 20% | 40% | 40% |
| default | 33% | 33% | 34% |

## 日志示例

正常运行日志：
```
INFO  Starting UAV Scheduler                        version=v0.2.0
INFO  Configuration loaded                          schedulerName=uav-scheduler algorithm=greed-nsgaii
INFO  Adaptive scheduler integration started
INFO  Pod scheduled successfully                    pod=app-7b4f5 node=uav-node-2 algorithm=greed-nsgaii
INFO  Deployment registered for coverage monitoring deployment=uav-coverage-app nodes=[uav-node-1 uav-node-2 ...]
```

覆盖率变化日志：
```
INFO  Coverage event received  deployment=uav-coverage-app event=CoverageDropped message="Coverage dropped by 15.3%"
INFO  Coverage event received  deployment=uav-coverage-app event=GreedyRequired
INFO  Greedy repair completed  deployment=uav-coverage-app newNodes=[uav-node-5]
INFO  Pod bound to node        pod=app-abc12 node=uav-node-5
```

重规划日志：
```
WARN  Node went offline        deployment=uav-coverage-app node=uav-node-3
INFO  Coverage event received  deployment=uav-coverage-app event=ReplanRequired message="Coverage: 58.0%, drop: 37.0%"
INFO  NSGA-II replan completed deployment=uav-coverage-app nodeCount=6 avgBattery=72.5
INFO  Pod deleted for reschedule  pod=app-xyz99
```

## 故障排查

### 1. 覆盖率检测不工作

检查 UAVMetrics 是否正常更新：
```bash
kubectl get uavmetrics -o wide
kubectl describe uavmetrics uav-node-1
```

### 2. 调度器无法绑定 Pod

检查 RBAC 权限：
```bash
kubectl auth can-i bind pods --as=system:serviceaccount:kube-system:uav-scheduler
```

### 3. 重规划频繁触发

调整阈值参数：
```yaml
env:
- name: ADAPTIVE_MAJOR_DROP
  value: "0.40"  # 提高到 40%
- name: ADAPTIVE_MIN_COVERAGE
  value: "0.60"  # 降低到 60%
```

## 监控指标

调度器暴露 Prometheus 指标：

```
# 当前覆盖率
uav_scheduler_coverage_ratio{deployment="app"} 0.85

# 贪心补充次数
uav_scheduler_greedy_repairs_total{deployment="app"} 3

# 重规划次数
uav_scheduler_replans_total{deployment="app"} 1

# 调度延迟
uav_scheduler_scheduling_duration_seconds{algorithm="greed-nsgaii"} 0.045
```

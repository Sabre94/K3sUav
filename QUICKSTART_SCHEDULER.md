# UAV Scheduler 快速入门

5 分钟快速上手 UAV 自定义调度器！

## 🚀 快速开始（本地测试）

### 步骤 1：编译调度器

```bash
make build-scheduler
```

编译成功后，二进制文件位于 `bin/uav-scheduler`（约 35MB）

### 步骤 2：本地运行调度器

```bash
# 使用默认算法（distance-based）
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
export ALGORITHM_NAME=distance-based
export LOG_LEVEL=debug
./bin/uav-scheduler
```

你会看到类似输出：
```
INFO[0000] Starting UAV Scheduler                       version=v0.1.0
INFO[0000] Configuration loaded                         algorithm=distance-based namespace=default schedulerName=uav-scheduler
INFO[0000] Registered algorithm: distance-based
INFO[0000] Registered algorithm: battery-aware
INFO[0000] Registered algorithm: network-latency
INFO[0000] Registered algorithm: composite
INFO[0000] Algorithm loaded                             algorithm=distance-based
INFO[0000] UAV client initialized
INFO[0000] Scheduler initialized
INFO[0000] Starting UAV Scheduler                       algorithm=distance-based schedulerName=uav-scheduler
INFO[0000] Watching for unscheduled pods...
```

### 步骤 3：创建测试 Pod

**在另一个终端**，创建一个使用自定义调度器的 Pod：

```bash
kubectl apply -f examples/test-pod.yaml
```

### 步骤 4：观察调度过程

在调度器终端，你会看到：
```
INFO[0005] Scheduling pod...                            namespace=default pod=test-uav-scheduled-pod
DEBU[0005] Fetched UAVMetrics                           nodeCount=5
DEBU[0005] Scoring completed                            algorithm=distance-based topScores=[...]
INFO[0005] Pod scheduled successfully                   duration=42 node=k3s-uav-pool-12 pod=test-uav-scheduled-pod reason="distance: 1.23km from target" score=98.78
```

验证 Pod 已调度：
```bash
kubectl get pod test-uav-scheduled-pod -o wide
```

输出示例：
```
NAME                      READY   STATUS    RESTARTS   AGE   IP            NODE
test-uav-scheduled-pod    1/1     Running   0          10s   10.42.1.123   k3s-uav-pool-12
```

## 🔌 切换算法测试

### 测试电池感知算法

```bash
# Ctrl+C 停止当前调度器
export ALGORITHM_NAME=battery-aware
export MIN_BATTERY=50.0  # 只调度到电量 >50% 的节点
./bin/uav-scheduler
```

### 测试网络延迟算法

```bash
export ALGORITHM_NAME=network-latency
export MAX_LATENCY=100.0  # 只调度到延迟 <100ms 的节点
./bin/uav-scheduler
```

### 测试组合算法

```bash
export ALGORITHM_NAME=composite
# 组合算法使用 60% 距离 + 40% 电池
./bin/uav-scheduler
```

## 📦 部署到集群

### 方法 1：快速部署（推荐）

```bash
# 1. 构建镜像（如果有 Docker）
make build-scheduler-image

# 2. 部署
make deploy-scheduler

# 3. 查看状态
make scheduler-status
kubectl get pods -l app=uav-scheduler

# 4. 查看日志
make scheduler-logs
```

### 方法 2：手动部署

```bash
# 1. 构建 Docker 镜像
docker build -f Dockerfile.scheduler -t uav-scheduler:v0.1.0 .

# 2. 导入到 K3s
docker save uav-scheduler:v0.1.0 | sudo k3s ctr images import -

# 3. 部署
kubectl apply -f deploy/scheduler-deployment.yaml

# 4. 验证
kubectl get pods -l app=uav-scheduler
kubectl logs -l app=uav-scheduler -f
```

## 🎯 使用场景示例

### 场景 1：将 Pod 调度到最近的节点

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: edge-app
  annotations:
    uav.scheduler/target-lat: "34.0522"
    uav.scheduler/target-lon: "-118.2437"
spec:
  schedulerName: uav-scheduler  # 使用自定义调度器
  containers:
  - name: app
    image: myapp:latest
```

**ConfigMap 配置**：
```yaml
ALGORITHM_NAME: "distance-based"
TARGET_LATITUDE: "34.0522"
TARGET_LONGITUDE: "-118.2437"
```

### 场景 2：只调度到电量充足的节点

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: long-running-task
spec:
  schedulerName: uav-scheduler
  containers:
  - name: task
    image: task-runner:latest
```

**ConfigMap 配置**：
```yaml
ALGORITHM_NAME: "battery-aware"
MIN_BATTERY: "60.0"  # 至少 60% 电量
```

### 场景 3：低延迟实时应用

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: realtime-stream
spec:
  schedulerName: uav-scheduler
  containers:
  - name: stream
    image: video-stream:latest
```

**ConfigMap 配置**：
```yaml
ALGORITHM_NAME: "network-latency"
MAX_LATENCY: "50.0"  # 最大 50ms 延迟
```

### 场景 4：综合考虑多因素

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: balanced-app
spec:
  schedulerName: uav-scheduler
  containers:
  - name: app
    image: balanced-app:latest
```

**ConfigMap 配置**：
```yaml
ALGORITHM_NAME: "composite"
# 默认：60% 距离 + 40% 电池
```

## 🐛 故障排查

### 问题：Pod 一直 Pending

```bash
# 1. 检查调度器是否运行
kubectl get pods -l app=uav-scheduler

# 2. 查看调度器日志
kubectl logs -l app=uav-scheduler

# 3. 检查 Pod 事件
kubectl describe pod <pod-name>

# 4. 确认 schedulerName 正确
kubectl get pod <pod-name> -o yaml | grep schedulerName
# 应该输出: schedulerName: uav-scheduler
```

### 问题：没有可用节点

```bash
# 检查 UAVMetrics
kubectl get uavmetrics -A

# 如果为空，说明 UAV Agent 未运行
kubectl get pods -l app=uav-agent
```

### 问题：调度到了不符合预期的节点

```bash
# 查看所有节点的分数（在调度器日志中）
kubectl logs -l app=uav-scheduler | grep "Scoring completed"

# 检查 UAVMetrics 数据
kubectl get uavmetrics -A -o custom-columns=\
NODE:.spec.nodeName,\
LAT:.spec.gps.latitude,\
LON:.spec.gps.longitude,\
BATTERY:.spec.battery.remainingPercent,\
LATENCY:.spec.network.latency
```

## 📊 对比测试

创建多个 Pod 观察不同算法的调度结果：

```bash
# 清理之前的测试
kubectl delete pod test-uav-scheduled-pod

# 测试 1：使用 distance-based
kubectl edit configmap uav-scheduler-config
# 修改 ALGORITHM_NAME: "distance-based"
kubectl rollout restart deployment uav-scheduler
kubectl apply -f examples/test-pod.yaml
kubectl get pod test-uav-scheduled-pod -o wide
# 记录调度到的节点

# 测试 2：使用 battery-aware
kubectl delete pod test-uav-scheduled-pod
kubectl edit configmap uav-scheduler-config
# 修改 ALGORITHM_NAME: "battery-aware"
kubectl rollout restart deployment uav-scheduler
kubectl apply -f examples/test-pod.yaml
kubectl get pod test-uav-scheduled-pod -o wide
# 对比调度结果
```

## 🎓 下一步

- 阅读完整文档：[SCHEDULER.md](./SCHEDULER.md)
- 开发自定义算法：参考 `pkg/scheduler/algorithm/` 中的示例
- 集成到 CI/CD：自动化部署和测试

## 💡 提示

1. **本地测试优先**：先在本地运行调度器，观察日志，理解调度逻辑
2. **日志级别**：使用 `LOG_LEVEL=debug` 查看详细的评分信息
3. **算法组合**：可以修改 `cmd/scheduler/main.go` 中的权重来调整组合算法
4. **性能监控**：调度器会记录每次调度的耗时（duration_ms）

---

**有问题？** 查看完整文档 [SCHEDULER.md](./SCHEDULER.md) 或提交 Issue。

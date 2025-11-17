# Pod 级别算法选择功能说明

## 📋 功能概述

现在每个 Pod 可以通过 **annotation** 自主选择使用哪种调度算法，而不是所有 Pod 共用一个固定算法。

## 🎯 支持的算法

| 算法名称 | annotation 值 | 说明 |
|---------|--------------|------|
| 基于距离 | `distance-based` | 调度到距离目标位置最近的节点 |
| 基于电池 | `battery-aware` | 调度到电量最充足的节点 |
| 基于网络 | `network-latency` | 调度到网络延迟最低的节点 |
| 组合算法 | `composite` | 综合考虑距离和电池（可配置权重） |

## 📝 Annotation 参数说明

### 通用参数

| Annotation Key | 说明 | 示例值 |
|---------------|------|--------|
| `uav.scheduler/algorithm` | 指定算法名称（**必需**） | `distance-based` |

### Distance-based 算法参数

| Annotation Key | 说明 | 默认值 | 示例值 |
|---------------|------|--------|--------|
| `uav.scheduler/target-lat` | 目标纬度 | 34.0522 | `"34.0522"` |
| `uav.scheduler/target-lon` | 目标经度 | -118.2437 | `"-118.2437"` |

### Battery-aware 算法参数

| Annotation Key | 说明 | 默认值 | 示例值 |
|---------------|------|--------|--------|
| `uav.scheduler/min-battery` | 最低电池百分比 | 30.0 | `"70.0"` |

### Network-latency 算法参数

| Annotation Key | 说明 | 默认值 | 示例值 |
|---------------|------|--------|--------|
| `uav.scheduler/max-latency` | 最大延迟（毫秒） | 200.0 | `"100.0"` |

### Composite 算法参数

| Annotation Key | 说明 | 默认值 | 示例值 |
|---------------|------|--------|--------|
| `uav.scheduler/target-lat` | 目标纬度（distance 子算法） | 34.0522 | `"34.0522"` |
| `uav.scheduler/target-lon` | 目标经度（distance 子算法） | -118.2437 | `"-118.2437"` |
| `uav.scheduler/min-battery` | 最低电池（battery 子算法） | 30.0 | `"40.0"` |
| `uav.scheduler/composite-weights` | 权重（格式: "距离,电池"） | "0.6,0.4" | `"0.7,0.3"` |

## 🚀 使用示例

### 示例 1: 基于距离调度

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: video-processor
  annotations:
    uav.scheduler/algorithm: "distance-based"
    uav.scheduler/target-lat: "34.0522"
    uav.scheduler/target-lon: "-118.2437"
spec:
  schedulerName: uav-scheduler
  containers:
  - name: processor
    image: nginx:latest
```

**效果**: Pod 会被调度到距离 (34.0522, -118.2437) 最近的节点

---

### 示例 2: 基于电池调度

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: long-running-task
  annotations:
    uav.scheduler/algorithm: "battery-aware"
    uav.scheduler/min-battery: "70.0"
spec:
  schedulerName: uav-scheduler
  containers:
  - name: task
    image: busybox:latest
    command: ["sleep", "3600"]
```

**效果**: Pod 会被调度到电量 ≥70% 的节点中电量最高的

---

### 示例 3: 基于网络延迟调度

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: realtime-app
  annotations:
    uav.scheduler/algorithm: "network-latency"
    uav.scheduler/max-latency: "100.0"
spec:
  schedulerName: uav-scheduler
  containers:
  - name: app
    image: nginx:latest
```

**效果**: Pod 会被调度到延迟 ≤100ms 的节点中延迟最低的

---

### 示例 4: 组合算法调度

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: balanced-app
  annotations:
    uav.scheduler/algorithm: "composite"
    uav.scheduler/target-lat: "34.0522"
    uav.scheduler/target-lon: "-118.2437"
    uav.scheduler/min-battery: "40.0"
    uav.scheduler/composite-weights: "0.7,0.3"  # 70% 距离 + 30% 电池
spec:
  schedulerName: uav-scheduler
  containers:
  - name: app
    image: nginx:latest
```

**效果**: 综合考虑距离（70%权重）和电池（30%权重）

---

### 示例 5: 使用默认算法

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: default-app
  # 不添加 uav.scheduler/algorithm annotation
spec:
  schedulerName: uav-scheduler
  containers:
  - name: app
    image: nginx:latest
```

**效果**: 使用调度器启动时配置的默认算法（通过环境变量 `ALGORITHM_NAME` 指定）

## 🧪 快速测试

### 1. 确保调度器正在运行

```bash
# 查看调度器状态
kubectl get pods -l app=uav-scheduler -n uav-system

# 查看调度器日志
kubectl logs -l app=uav-scheduler -n uav-system -f
```

### 2. 部署测试 Pod

```bash
# 测试距离算法
kubectl apply -f examples/test-pod-distance.yaml

# 测试电池算法
kubectl apply -f examples/test-pod-battery.yaml

# 测试网络算法
kubectl apply -f examples/test-pod-network.yaml

# 测试组合算法
kubectl apply -f examples/test-pod-composite.yaml

# 测试默认算法
kubectl apply -f examples/test-pod-default.yaml
```

### 3. 查看调度结果

```bash
# 查看 Pod 被调度到哪个节点
kubectl get pods -o wide

# 查看调度日志（包含算法选择信息）
kubectl logs -l app=uav-scheduler -n uav-system | grep "Algorithm selected"
```

### 4. 清理测试 Pod

```bash
kubectl delete pod video-processor long-running-task realtime-app balanced-app default-app
```

## 📊 调度日志示例

当 Pod 部署时，调度器日志会显示：

```
time="2025-11-17 10:00:00" level=debug msg="Algorithm selected for pod" pod=video-processor algorithm=distance-based
time="2025-11-17 10:00:00" level=info msg="Pod scheduled successfully" pod=video-processor node=k3s-node-1 score=85.32 reason="distance: 2.45km from target" duration=45
```

## 🔍 故障排查

### 问题 1: Pod 一直处于 Pending 状态

**检查步骤**:
1. 查看调度器是否运行: `kubectl get pods -l app=uav-scheduler -n uav-system`
2. 查看调度器日志: `kubectl logs -l app=uav-scheduler -n uav-system`
3. 检查 UAVMetrics 是否存在: `kubectl get uavmetrics`
4. 确认 Pod 的 `schedulerName` 是 `uav-scheduler`

### 问题 2: 算法参数没有生效

**检查步骤**:
1. 确认 annotation key 拼写正确（注意大小写）
2. 确认参数值是字符串格式（需要用引号）
3. 查看调度器日志中的算法选择信息

### 问题 3: 没有满足条件的节点

**可能原因**:
- `min-battery` 设置过高，所有节点电量都不足
- `max-latency` 设置过低，所有节点延迟都超标
- UAVMetrics 数据缺失或不完整

**解决方法**:
- 适当调整参数阈值
- 检查 Agent 是否正常采集数据

## 🎯 最佳实践

1. **根据应用特性选择算法**
   - 视频流处理 → `distance-based`
   - 长时间后台任务 → `battery-aware`
   - 实时通信应用 → `network-latency`
   - 综合需求 → `composite`

2. **合理设置阈值**
   - 电池阈值不要过高（推荐 30%-50%）
   - 延迟阈值根据应用需求设置（推荐 100-200ms）

3. **使用默认算法作为兜底**
   - 如果不确定用哪个算法，不添加 annotation
   - 调度器会使用默认算法进行调度

4. **监控调度结果**
   - 定期查看调度日志
   - 根据实际效果调整参数

## 📚 相关文档

- [SCHEDULER.md](../SCHEDULER.md) - 调度器详细文档
- [ARCHITECTURE.md](../ARCHITECTURE.md) - 系统架构文档
- [README.md](../README.md) - 项目主文档

# Pod 级别算法选择功能 - 更新日志

## 📅 更新时间
2025-11-17

## 🎯 功能概述

**新增功能**: Pod 级别的调度算法选择

**原有行为**: 调度器启动时指定一个算法，所有 Pod 都使用同一个算法

**新行为**: 每个 Pod 可以通过 annotation 自主选择使用哪种调度算法

## 📝 修改内容

### 1. 新增文件

#### `pkg/scheduler/algorithm/factory.go`
- **功能**: 算法工厂，根据 Pod annotation 动态创建算法实例
- **核心方法**:
  - `CreateFromPod()`: 从 Pod annotation 创建算法
  - `createDistanceBased()`: 创建距离算法
  - `createBatteryAware()`: 创建电池算法
  - `createNetworkLatency()`: 创建网络算法
  - `createComposite()`: 创建组合算法

#### 测试 Pod 示例文件
- `examples/test-pod-distance.yaml` - 基于距离算法的测试 Pod
- `examples/test-pod-battery.yaml` - 基于电池算法的测试 Pod
- `examples/test-pod-network.yaml` - 基于网络算法的测试 Pod
- `examples/test-pod-composite.yaml` - 基于组合算法的测试 Pod
- `examples/test-pod-default.yaml` - 使用默认算法的测试 Pod

#### 文档
- `examples/POD_LEVEL_SCHEDULING.md` - Pod 级别调度功能完整说明文档

### 2. 修改文件

#### `pkg/scheduler/scheduler.go`
**修改点**:
1. **Scheduler 结构体** (第 22-30 行)
   ```go
   type Scheduler struct {
       // ... 原有字段
       algorithm     algorithm.SchedulingAlgorithm // 默认算法
       algoFactory   *algorithm.AlgorithmFactory   // 新增：算法工厂
       log           *logrus.Logger
   }
   ```

2. **NewScheduler 函数** (第 85-93 行)
   ```go
   return &Scheduler{
       // ... 原有字段
       algoFactory: algorithm.NewAlgorithmFactory(), // 新增：初始化工厂
       // ...
   }
   ```

3. **schedulePod 函数** (第 170-244 行)
   - 添加了算法选择逻辑（第 186-197 行）
   ```go
   // 根据 Pod annotation 选择算法
   selectedAlgo, err := s.algoFactory.CreateFromPod(pod, s.algorithm)
   if err != nil {
       s.log.WithError(err).Warn("Failed to create algorithm, using default")
       selectedAlgo = s.algorithm
   }

   s.log.WithFields(logrus.Fields{
       "pod":       pod.Name,
       "algorithm": selectedAlgo.Name(),
   }).Debug("Algorithm selected for pod")
   ```

   - 将 `s.algorithm` 替换为 `selectedAlgo` 用于过滤和评分

## 🔧 支持的 Annotation

### 算法选择

| Annotation | 说明 | 可选值 |
|-----------|------|--------|
| `uav.scheduler/algorithm` | 指定算法 | `distance-based`, `battery-aware`, `network-latency`, `composite` |

### 算法参数

| Annotation | 适用算法 | 说明 | 默认值 |
|-----------|---------|------|--------|
| `uav.scheduler/target-lat` | distance-based, composite | 目标纬度 | 34.0522 |
| `uav.scheduler/target-lon` | distance-based, composite | 目标经度 | -118.2437 |
| `uav.scheduler/min-battery` | battery-aware, composite | 最低电池% | 30.0 |
| `uav.scheduler/max-latency` | network-latency | 最大延迟ms | 200.0 |
| `uav.scheduler/composite-weights` | composite | 权重 "距离,电池" | "0.6,0.4" |

## 📊 使用示例

### 场景 1: 不同应用使用不同算法

```yaml
# Pod A: 视频处理应用 - 使用距离算法
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
  # ...
---
# Pod B: 长时间任务 - 使用电池算法
apiVersion: v1
kind: Pod
metadata:
  name: long-task
  annotations:
    uav.scheduler/algorithm: "battery-aware"
    uav.scheduler/min-battery: "70.0"
spec:
  schedulerName: uav-scheduler
  # ...
```

### 场景 2: 向后兼容

```yaml
# Pod C: 不指定算法 - 使用默认算法
apiVersion: v1
kind: Pod
metadata:
  name: legacy-app
  # 没有算法 annotation
spec:
  schedulerName: uav-scheduler
  # ...
```

**效果**: Pod C 会使用调度器启动时配置的默认算法（环境变量 `ALGORITHM_NAME`）

## 🎓 技术实现

### 设计模式

1. **工厂模式 (Factory Pattern)**
   - `AlgorithmFactory` 负责创建算法实例
   - 根据 Pod annotation 动态选择和配置算法

2. **策略模式 (Strategy Pattern)**
   - 每个算法实现 `SchedulingAlgorithm` 接口
   - 调度器通过接口调用算法，实现解耦

### 执行流程

```
schedulePod(pod)
   │
   ├─► 1. 获取 UAVMetrics
   │
   ├─► 2. 【新增】从 Pod annotation 选择算法
   │      algoFactory.CreateFromPod(pod, defaultAlgo)
   │      │
   │      ├─ 读取 uav.scheduler/algorithm
   │      ├─ 读取算法参数 annotation
   │      └─ 创建算法实例
   │
   ├─► 3. 使用选定的算法过滤节点
   │      selectedAlgo.Filter(...)
   │
   ├─► 4. 使用选定的算法评分
   │      selectedAlgo.Score(...)
   │
   └─► 5. 绑定到最高分节点
```

## ✅ 兼容性

- ✅ **向后兼容**: Pod 不指定 annotation 时，使用默认算法
- ✅ **优雅降级**: annotation 解析失败时，回退到默认算法
- ✅ **现有部署无需修改**: 所有现有的 Pod 定义都能正常工作

## 🧪 测试验证

### 编译测试
```bash
cd /home/ubuntu/K3sUav
export PATH=$PATH:/usr/local/go/bin
go build -o bin/uav-scheduler ./cmd/scheduler/
```
✅ 编译成功

### 功能测试步骤

1. **部署测试 Pod**
   ```bash
   kubectl apply -f examples/test-pod-distance.yaml
   kubectl apply -f examples/test-pod-battery.yaml
   kubectl apply -f examples/test-pod-composite.yaml
   ```

2. **查看调度结果**
   ```bash
   kubectl get pods -o wide
   ```

3. **查看调度日志**
   ```bash
   kubectl logs -l app=uav-scheduler -n uav-system | grep "Algorithm selected"
   ```

预期日志输出：
```
level=debug msg="Algorithm selected for pod" pod=video-processor algorithm=distance-based
level=debug msg="Algorithm selected for pod" pod=long-task algorithm=battery-aware
level=debug msg="Algorithm selected for pod" pod=balanced-app algorithm=composite
```

## 🚀 部署步骤

### 1. 重新构建 Scheduler 镜像

```bash
# 构建镜像
docker build -f Dockerfile.scheduler -t uav-scheduler:v0.2.0 .

# 导入到 K3s
docker save uav-scheduler:v0.2.0 | sudo k3s ctr images import -
```

### 2. 更新 Scheduler Deployment

编辑 `deploy/scheduler-deployment.yaml`，将镜像版本更新为 `v0.2.0`:

```yaml
spec:
  template:
    spec:
      containers:
      - name: scheduler
        image: uav-scheduler:v0.2.0  # 更新版本号
```

### 3. 重新部署

```bash
kubectl apply -f deploy/scheduler-deployment.yaml
kubectl rollout restart deployment uav-scheduler -n uav-system
```

### 4. 验证部署

```bash
kubectl get pods -l app=uav-scheduler -n uav-system
kubectl logs -l app=uav-scheduler -n uav-system -f
```

## 📖 相关文档

- `examples/POD_LEVEL_SCHEDULING.md` - Pod 级别调度完整使用指南
- `SCHEDULER.md` - 调度器详细文档
- `ARCHITECTURE.md` - 系统架构文档

## 👥 贡献者

- 实现者: Claude Code
- 需求方: K3sUav Team

---

**版本**: v0.2.0
**更新时间**: 2025-11-17

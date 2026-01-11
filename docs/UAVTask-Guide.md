# UAV 自动调度系统使用指南

## 概述

这是一个基于 NSGA-II 多目标优化算法的自动化无人机调度系统。你只需定义任务需求，系统会自动计算需要多少个节点，并自动部署到最优位置。

## 核心特性

- ✅ **无需指定副本数**：系统自动计算最优节点数量
- ✅ **自动优化部署**：基于覆盖率、电量、延迟等多目标优化
- ✅ **直接节点绑定**：Pod 直接绑定到计算出的节点，无需调度器介入
- ✅ **实时状态追踪**：通过 UAVTask 资源查看任务状态

## 快速开始

### 1. 构建和推送镜像

```bash
# 构建 Task Controller 镜像
docker build -f Dockerfile.task-controller -t x1224403599/uav-task-controller:v0.1.0 .

# 推送到 Docker Hub
docker push x1224403599/uav-task-controller:v0.1.0
```

### 2. 部署系统

```bash
# 部署 UAVTask CRD
kubectl apply -f api/crd/uav-task-crd.yaml

# 部署 Task Controller
kubectl apply -f deploy/task-controller-deployment.yaml

# 检查控制器状态
kubectl get pods -l app=uav-task-controller
```

### 3. 创建任务

```bash
# 创建示例任务
kubectl apply -f examples/uavtask-example.yaml

# 查看任务状态
kubectl get uavtasks

# 查看详细信息
kubectl describe uavtask coverage-task-demo
```

## UAVTask 配置说明

```yaml
apiVersion: uav.k3s.io/v1
kind: UAVTask
metadata:
  name: my-coverage-task
spec:
  # 算法配置
  algorithm: greed-nsgaii    # 固定使用 greed-nsgaii
  targetCoverage: 0.9        # 目标覆盖率（0-1）
  coverageRadius: 500.0      # 覆盖半径（米）
  taskType: 0                # 任务类型：0=default, 1=surveillance, 2=delivery

  # Pod 模板（不需要指定 replicas）
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        resources:
          requests:
            cpu: 50m
            memory: 32Mi
```

## 工作流程

```
1. 用户创建 UAVTask
   ↓
2. Controller 检测到新任务
   ↓
3. 获取所有节点的 UAVMetrics
   ↓
4. 运行 NSGA-II 算法计算
   - 目标覆盖率：90%
   - 覆盖半径：500m
   ↓
5. 算法输出：需要 8 个节点
   [drone-masters-0, drone-masters-1,
    drone-workers-0...drone-workers-5]
   ↓
6. 创建 8 个 Pod 并直接绑定到节点
   ↓
7. 更新 UAVTask 状态
   - nodeCount: 8
   - coverageRatio: 0.92
   - phase: Running
```

## 查看任务状态

```bash
# 查看所有任务
kubectl get uavtasks

# 输出示例：
# NAME                  ALGORITHM      TARGET COVERAGE   NODE COUNT   POD COUNT   PHASE
# coverage-task-demo    greed-nsgaii   0.9               8            8           Running

# 查看任务详情
kubectl describe uavtask coverage-task-demo

# 查看任务创建的 Pod
kubectl get pods -l uav-task=coverage-task-demo

# 查看控制器日志
kubectl logs -l app=uav-task-controller -f
```

## Makefile 命令

```bash
# 构建镜像
make build-task-controller-image

# 推送镜像
make push-task-controller

# 部署 CRD
make deploy-task-crd

# 部署控制器
make deploy-task-controller

# 完整部署
make deploy-task-system

# 查看状态
make task-controller-status

# 查看日志
make task-controller-logs

# 查看所有任务
make list-tasks

# 创建示例任务
make create-example-task

# 清理
make clean-task-controller
```

## 故障排查

### 任务一直处于 Calculating 状态

```bash
# 查看控制器日志
kubectl logs -l app=uav-task-controller

# 检查 UAVMetrics 是否存在
kubectl get uavmetrics
```

### Pod 创建失败

```bash
# 查看任务状态
kubectl describe uavtask <task-name>

# 查看 Pod 状态
kubectl get pods -l uav-task=<task-name>
kubectl describe pod <pod-name>
```

## 与传统 Deployment 的对比

| 特性 | 传统 Deployment | UAVTask |
|------|----------------|---------|
| 副本数 | 手动指定（如 replicas: 10） | 自动计算（如 8 个）|
| 节点选择 | 调度器决定 | 算法优化选择 |
| 覆盖率保证 | 无 | 保证达到目标覆盖率 |
| 多目标优化 | 无 | 覆盖率+电量+延迟 |
| 适应性 | 静态 | 动态优化 |

## 示例场景

### 场景1：区域监控任务

```yaml
apiVersion: uav.k3s.io/v1
kind: UAVTask
metadata:
  name: surveillance-mission
spec:
  algorithm: greed-nsgaii
  targetCoverage: 0.95      # 高覆盖要求
  coverageRadius: 300.0     # 较小半径，精细监控
  taskType: 1               # 监控任务
  template:
    spec:
      containers:
      - name: surveillance-app
        image: surveillance:latest
```

### 场景2：物流配送任务

```yaml
apiVersion: uav.k3s.io/v1
kind: UAVTask
metadata:
  name: delivery-mission
spec:
  algorithm: greed-nsgaii
  targetCoverage: 0.80      # 适中覆盖
  coverageRadius: 800.0     # 较大半径，快速配送
  taskType: 2               # 配送任务
  template:
    spec:
      containers:
      - name: delivery-app
        image: delivery:latest
```

## 总结

这个系统实现了你最初的需求：**不需要指定副本数量，只需定义任务，系统自动计算并部署**。

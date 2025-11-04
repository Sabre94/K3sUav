# UAV Agent 部署指南

## 🎯 部署方式对比

### 方式 A：Kubernetes DaemonSet 部署（推荐）✅

**优点：**
- ✅ Kubernetes 原生部署
- ✅ 自动在所有节点运行
- ✅ 自动重启和健康检查
- ✅ 统一日志管理
- ✅ 易于扩展和维护

**适用场景：** 生产环境、长期运行

### 方式 B：直接运行二进制文件

**优点：**
- ✅ 快速测试
- ✅ 易于调试
- ✅ 不需要构建镜像

**缺点：**
- ❌ 需要手动管理每个节点
- ❌ 没有自动重启
- ❌ 日志分散

**适用场景：** 开发调试

---

## 📦 方式 A：DaemonSet 部署（推荐）

### 前置条件

```bash
# 1. 检查集群状态
kubectl get nodes

# 2. 检查 Docker（用于构建镜像）
docker version

# 如果没有 Docker，可以先编译后手动测试
```

### 步骤 1：构建镜像

```bash
# 方法 1：使用 Makefile（推荐）
make build-image

# 方法 2：手动构建
docker build -t uav-agent:v0.1.0 .
docker save uav-agent:v0.1.0 | sudo k3s ctr images import -
```

### 步骤 2：部署

```bash
# 一键部署（推荐）
make deploy

# 或者手动部署
./deploy/deploy.sh

# 或者分步部署
kubectl apply -f api/crd/uav-metrics-crd.yaml
kubectl apply -f deploy/agent-daemonset.yaml
```

### 步骤 3：验证部署

```bash
# 查看状态
make status

# 或者手动查看
kubectl get pods -l app=uav-agent -o wide
kubectl get uavmetrics -A
```

### 步骤 4：查看日志

```bash
# 所有 Agent 日志
make logs

# 或者手动
kubectl logs -l app=uav-agent -f

# 查看特定节点
kubectl get pods -l app=uav-agent -o wide
kubectl logs <pod-name> -f
```

### 步骤 5：查看数据

```bash
# 实时监控
watch -n 2 'kubectl get uavmetrics -A'

# 查看详细数据
kubectl get uavmetrics uav-<node-name> -o yaml

# 示例
kubectl get uavmetrics uav-k3s-uav-pool-master-0 -o yaml
```

---

## 🚀 方式 B：快速测试（不构建镜像）

如果暂时不想构建 Docker 镜像，可以直接运行二进制文件：

### 在 Master 节点测试

```bash
# 1. 部署 CRD
kubectl apply -f api/crd/uav-metrics-crd.yaml

# 2. 编译
make build

# 3. 运行（前台）
export NODE_NAME=$(hostname)
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
./bin/uav-agent
```

### 在多个节点测试

你需要在每个节点上执行：

```bash
# 1. 复制二进制文件到节点
NODE="k3s-uav-pool-12"
scp bin/uav-agent $NODE:/tmp/

# 2. SSH 到节点运行
ssh $NODE
export NODE_NAME=$(hostname)
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
/tmp/uav-agent
```

**缺点：** 需要手动管理每个节点，不推荐长期使用。

---

## 🔍 故障排查

### 问题 1：Pod 无法启动

```bash
# 查看 Pod 详情
kubectl describe pod <pod-name>

# 查看事件
kubectl get events -n default --sort-by='.lastTimestamp'

# 常见原因：
# - 镜像未找到：需要先 build-image
# - 权限问题：检查 ServiceAccount
# - 资源不足：检查节点资源
```

### 问题 2：CRD 创建失败

```bash
# 查看 Agent 日志
kubectl logs <pod-name>

# 常见错误：
# - "tls: failed to verify certificate": KUBECONFIG 问题（容器内会自动解决）
# - "forbidden": ServiceAccount 权限不足
```

### 问题 3：数据不更新

```bash
# 1. 检查 Agent 是否运行
kubectl get pods -l app=uav-agent

# 2. 查看日志
kubectl logs -l app=uav-agent --tail=50

# 3. 检查 CRD 权限
kubectl auth can-i create uavmetrics --as=system:serviceaccount:default:uav-agent
```

---

## 📊 验证数据收集

### 基本验证

```bash
# 1. 查看有多少个 UAV 在线
kubectl get uavmetrics -A | wc -l

# 应该等于节点数量 + 1（标题行）

# 2. 查看电池数据
kubectl get uavmetrics -o custom-columns=\
NAME:.metadata.name,\
NODE:.spec.nodeName,\
BATTERY:.spec.battery.remainingPercent,\
STATUS:.spec.health.status

# 3. 查看 GPS 数据
kubectl get uavmetrics -o custom-columns=\
NAME:.metadata.name,\
LAT:.spec.gps.latitude,\
LON:.spec.gps.longitude,\
SATS:.spec.gps.satellites
```

### 详细验证

```bash
# 查看完整数据
kubectl get uavmetrics <name> -o yaml

# 检查字段
kubectl get uavmetrics <name> -o json | jq '.spec | keys'

# 应该看到：
# - nodeName
# - gps
# - battery
# - flight
# - network
# - performance
# - health
# - metadata
```

---

## 🗑️ 卸载

```bash
# 删除 DaemonSet（保留 CRD 和数据）
make clean

# 完全删除（包括 CRD 和所有数据）
make clean-all

# 或者手动
kubectl delete -f deploy/agent-daemonset.yaml
kubectl delete -f api/crd/uav-metrics-crd.yaml
```

---

## 📝 常用命令速查

```bash
# 编译
make build

# 构建镜像
make build-image

# 部署
make deploy

# 查看状态
make status

# 查看日志
make logs

# 本地测试
make test-local

# 清理
make clean

# 帮助
make help
```

---

## 🎯 下一步

部署成功后，你可以：

1. **监控数据**：`watch -n 2 'kubectl get uavmetrics -A'`
2. **开发调度器**：读取 UAVMetrics 数据进行任务分配
3. **集成无人机模拟器**：替换 collector 中的模拟数据
4. **添加 Web UI**：可视化展示 UAV 数据

---

## ❓ 常见问题

**Q: 为什么要用 DaemonSet？**
A: DaemonSet 确保每个节点上都运行一个 Pod，适合收集每个节点的数据。

**Q: 可以不用 Docker 吗？**
A: 可以，用方式 B 直接运行二进制文件，但不推荐长期使用。

**Q: 如何添加更多节点？**
A: K3s 添加新节点后，DaemonSet 会自动在新节点上启动 Agent。

**Q: 数据存在哪里？**
A: 存储在 Kubernetes etcd 中（作为 UAVMetrics CRD）。

**Q: Agent 挂了怎么办？**
A: Kubernetes 会自动重启 Pod（DaemonSet 的 `restartPolicy: Always`）。

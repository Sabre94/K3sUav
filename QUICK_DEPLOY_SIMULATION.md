# 快速部署指南 - 仿真模式

## 前提条件检查 ✅

```bash
# 1. 集群状态
kubectl get nodes
# 预期: 12个节点全部Ready

# 2. CRD已安装
kubectl get crd uavmetrics.uav.k3s.io
# 预期: 显示CRD信息

# 3. 仿真数据文件
cat /data/sim/current.json
# 预期: 显示JSON格式的仿真数据
```

## 快速部署步骤

### 步骤1: 安装RBAC权限（仅需一次）

```bash
cd /home/ubuntu/DevUav/K3sUav

# 提取RBAC部分并部署
kubectl apply -f - <<'EOF'
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: uav-agent
  namespace: default
  labels:
    app: uav-agent
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: uav-agent
  labels:
    app: uav-agent
rules:
  - apiGroups: ["uav.k3s.io"]
    resources: ["uavmetrics"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["uav.k3s.io"]
    resources: ["uavmetrics/status"]
    verbs: ["get", "update", "patch"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: uav-agent
  labels:
    app: uav-agent
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: uav-agent
subjects:
  - kind: ServiceAccount
    name: uav-agent
    namespace: default
EOF
```

验证:
```bash
kubectl get sa uav-agent
kubectl get clusterrole uav-agent
kubectl get clusterrolebinding uav-agent
```

### 步骤2: 重新编译Agent（需要Go编译器）

```bash
cd /home/ubuntu/DevUav/K3sUav

# 如果有Go编译器
go build -o bin/uav-agent-v0.3 ./cmd/agent/

# 验证编译
./bin/uav-agent-v0.3 --help 2>&1 || echo "二进制文件已生成"
```

**如果没有Go编译器**:
```bash
# 在有Go的机器上交叉编译
GOOS=linux GOARCH=arm64 go build -o bin/uav-agent-v0.3 ./cmd/agent/
# 然后复制到目标机器
```

### 步骤3: 构建Docker镜像

```bash
# 方案A: 使用Docker
docker build -t uav-agent:v0.3.0-sim -f Dockerfile .
docker save uav-agent:v0.3.0-sim | sudo k3s ctr images import -

# 方案B: 直接导入K3s
# 先创建简单的Dockerfile
cat > Dockerfile.sim <<'EOF'
FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY bin/uav-agent-v0.3 /usr/local/bin/uav-agent
ENTRYPOINT ["/usr/local/bin/uav-agent"]
EOF

# 构建并导入
docker build -t uav-agent:v0.3.0-sim -f Dockerfile.sim .
docker save uav-agent:v0.3.0-sim | sudo k3s ctr images import -

# 验证镜像
sudo k3s crictl images | grep uav-agent
```

### 步骤4: 更新DaemonSet配置

修改 `deploy/agent-daemonset-simulation-test.yaml` 中的镜像:

```bash
# 使用sed直接修改
sed -i 's|image: x1224403599/uav-agent:v0.1.0|image: uav-agent:v0.3.0-sim|g' \
  deploy/agent-daemonset-simulation-test.yaml

# 验证修改
grep "image:" deploy/agent-daemonset-simulation-test.yaml
```

### 步骤5: 部署Agent

```bash
# 部署仿真测试DaemonSet
kubectl apply -f deploy/agent-daemonset-simulation-test.yaml

# 等待Pod启动
kubectl wait --for=condition=ready pod -l app=uav-agent-sim --timeout=60s

# 查看状态
kubectl get pods -l app=uav-agent-sim -o wide
```

### 步骤6: 验证部署

```bash
# 1. 检查Pod日志（应显示仿真模式）
kubectl logs -l app=uav-agent-sim --tail=50 | grep -i simulation

# 期望输出:
# "simulationMode": true
# "Simulation collector initialized"
# "dataPath": "/data/sim/current.json"

# 2. 查看UAVMetrics资源
kubectl get uavmetrics

# 3. 查看详细数据（选择一个节点）
NODE_NAME=$(kubectl get nodes -o name | head -1 | cut -d/ -f2)
kubectl get uavmetrics uav-${NODE_NAME} -o yaml

# 4. 验证新字段存在
kubectl get uavmetrics uav-${NODE_NAME} -o jsonpath='{.spec.position}' | jq .
kubectl get uavmetrics uav-${NODE_NAME} -o jsonpath='{.spec.velocity}' | jq .
kubectl get uavmetrics uav-${NODE_NAME} -o jsonpath='{.spec.simulation}' | jq .
```

### 步骤7: 实时监控

```bash
# 方法1: Watch命令
watch -n 2 'kubectl get uavmetrics -o wide'

# 方法2: 持续查看日志
kubectl logs -l app=uav-agent-sim -f

# 方法3: 监控特定节点的数据更新
watch -n 2 "kubectl get uavmetrics uav-drone-workers-0 -o jsonpath='{.spec.simulation.timeStep}'"
```

## 快速测试数据更新

```bash
# 修改仿真数据文件
cat > /data/sim/current.json <<'EOF'
{
  "vm_id": "drone_test_001",
  "simulation_id": "test_scenario",
  "timestamp": "2024-01-01T12:00:00Z",
  "time_step": 999,
  "position": { "x": 100.0, "y": 200.0, "z": 50.0 },
  "velocity": { "vx": 5.0, "vy": 10.0, "vz": 0.5 },
  "geodetic": {
    "latitude": 39.9042,
    "longitude": 116.4074,
    "altitude": 100.0
  },
  "battery_level": 0.85,
  "heading": 90.0,
  "altitude_agl": 50.0
}
EOF

# 等待10秒（默认采集间隔）
sleep 10

# 检查UAVMetrics是否更新
kubectl get uavmetrics -o json | grep -i "time_step"
# 应显示: "timeStep": 999
```

## 清理

```bash
# 删除测试DaemonSet
kubectl delete -f deploy/agent-daemonset-simulation-test.yaml

# 删除UAVMetrics资源
kubectl delete uavmetrics --all

# （可选）删除RBAC
kubectl delete clusterrolebinding uav-agent
kubectl delete clusterrole uav-agent
kubectl delete sa uav-agent

# （可选）删除CRD
kubectl delete crd uavmetrics.uav.k3s.io
```

## 故障排查速查表

| 问题 | 检查命令 | 可能原因 |
|------|---------|---------|
| Pod未启动 | `kubectl describe pod <pod>` | 镜像拉取失败、权限问题 |
| 日志无输出 | `kubectl logs <pod>` | 配置错误、文件路径错误 |
| UAVMetrics未创建 | `kubectl auth can-i create uavmetrics.uav.k3s.io --as=system:serviceaccount:default:uav-agent` | RBAC权限不足 |
| 数据未更新 | `cat /data/sim/current.json` | 文件不存在或格式错误 |
| CRD字段缺失 | `kubectl get crd uavmetrics.uav.k3s.io -o yaml \| grep -A 5 position` | CRD版本过旧 |

## 成功标志

部署成功后应看到:

✅ 12个agent Pod全部Running
✅ 每个节点对应一个UAVMetrics资源
✅ UAVMetrics包含position、velocity、simulation字段
✅ 日志显示"Simulation collector initialized"
✅ timeStep字段每10秒更新一次

## 下一步

1. **多无人机仿真**: 为每个节点准备独立的仿真数据文件
2. **调度器测试**: 部署uav-scheduler测试调度算法
3. **性能测试**: 压力测试仿真数据更新频率
4. **集成测试**: 测试RL、NSGA-II等调度算法与仿真数据的配合

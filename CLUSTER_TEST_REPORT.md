# K3s集群测试报告

## 测试时间
2026-01-11

## 集群状态检查结果

### ✅ 1. Kubernetes集群连接
**状态**: 正常运行

```
控制平面: https://127.0.0.1:6443
K3s版本: v1.34.3+k3s1
```

**节点信息**:
- **Master节点**: 3个 (drone-masters-0/1/2)
- **Worker节点**: 9个 (drone-workers-0 至 drone-workers-8)
- **总计**: 12个节点，全部状态为 Ready

### ✅ 2. UAVMetrics CRD安装
**状态**: 已成功安装

```bash
$ kubectl get crd | grep uav
uavmetrics.uav.k3s.io     2026-01-11T06:XX:XXZ
```

**CRD详情**:
- API Group: `uav.k3s.io`
- API Version: `v1alpha1`
- Kind: `UAVMetrics`
- Scope: Namespaced
- Short Names: `uav`, `uavs`

**新增字段** (V0.3 更新):
- ✅ `position` - 笛卡尔坐标系位置
- ✅ `velocity` - 速度向量
- ✅ `simulation` - 仿真元数据

### ✅ 3. 仿真数据文件
**状态**: 存在且格式正确

**文件路径**: `/data/sim/current.json`

**数据示例**:
```json
{
  "vm_id": "drone_000",
  "simulation_id": "drone_swarm_v1",
  "timestamp": "2024-01-01T00:00:40Z",
  "time_step": 8,
  "position": { "x": -438.43, "y": -2.34, "z": 10 },
  "velocity": { "vx": -13.31, "vy": -6.92, "vz": 0 },
  "geodetic": {
    "latitude": 39.904178789070706,
    "longitude": 116.40227293294954,
    "altitude": 60.015048624016345
  },
  "battery_level": 0.9675859499998042,
  "heading": 242.53296849210184,
  "altitude_agl": 10
}
```

### ❌ 4. UAV Agent部署
**状态**: 未部署

当前集群中没有任何UAV相关的Pod或DaemonSet。

### ⚠️ 5. Agent二进制文件
**状态**: 存在旧版本，需要重新编译

**现有文件**:
- `/home/ubuntu/DevUav/K3sUav/bin/uav-agent` (15MB, ARM64)
- 编译时间: 2026-01-11 05:10 (代码修改之前)

**问题**: 现有二进制不包含仿真功能代码

### ✅ 6. RBAC配置
**状态**: 配置文件已准备好

RBAC资源定义在 `deploy/agent-daemonset.yaml` 中:
- ServiceAccount: `uav-agent`
- ClusterRole: `uav-agent` (包含UAVMetrics的完整权限)
- ClusterRoleBinding: `uav-agent`

---

## 代码修改总结

### 已完成的修改 ✅

1. **数据模型扩展** (`pkg/models/types.go`)
   - 新增 `PositionData` 结构体
   - 新增 `VelocityData` 结构体
   - 新增 `SimulationInfo` 结构体

2. **CRD定义更新** (`api/crd/uav-metrics-crd.yaml`)
   - 添加 position、velocity、simulation 字段规范
   - 已应用到K3s集群

3. **仿真采集器** (`pkg/collector/simulation_collector.go`)
   - 从JSON文件读取仿真数据
   - 自动格式转换和字段映射
   - 集成健康检查

4. **配置系统** (`pkg/config/config.go`)
   - 新增 `SimulationConfig` 配置
   - 支持环境变量控制

5. **Agent主程序** (`cmd/agent/main.go`)
   - 动态选择采集器（仿真/真实）
   - 统一接口设计

6. **部署配置** (`deploy/agent-daemonset-simulation-test.yaml`)
   - 仿真模式测试配置
   - 挂载 `/data/sim` 目录

---

## 下一步操作

### 方案A: 重新编译并测试 (推荐)

#### 1. 重新编译agent
```bash
cd /home/ubuntu/DevUav/K3sUav

# 需要Go编译器
go build -o bin/uav-agent ./cmd/agent/

# 如果是跨架构编译
GOOS=linux GOARCH=arm64 go build -o bin/uav-agent ./cmd/agent/
```

#### 2. 构建Docker镜像
```bash
# 方法1: 使用Dockerfile
docker build -t uav-agent:v0.3.0-simulation .

# 方法2: 导入到K3s
docker save uav-agent:v0.3.0-simulation | sudo k3s ctr images import -
```

#### 3. 部署RBAC
```bash
# 只需部署一次
kubectl apply -f deploy/agent-daemonset.yaml --server-side --force-conflicts
# 注意：这会部署RBAC和旧版agent，我们需要先删除DaemonSet
kubectl delete daemonset uav-agent --ignore-not-found
```

#### 4. 部署仿真测试agent
```bash
# 修改镜像标签后部署
kubectl apply -f deploy/agent-daemonset-simulation-test.yaml
```

#### 5. 验证部署
```bash
# 查看Pod状态
kubectl get pods -l app=uav-agent-sim -o wide

# 查看日志（应显示"Simulation collector initialized"）
kubectl logs -l app=uav-agent-sim -f

# 查看UAVMetrics资源
kubectl get uavmetrics -A

# 查看详细数据（应包含position、velocity、simulation字段）
kubectl get uavmetrics <node-name> -o yaml
```

---

### 方案B: 使用现有镜像测试 (如果没有Go编译器)

**限制**: 现有镜像不包含仿真代码，需要等待重新编译

临时解决方案:
1. 在宿主机上直接运行重新编译的二进制
2. 或者在有Go环境的机器上交叉编译

---

## 测试检查清单

部署后验证以下内容:

- [ ] Pod成功启动并运行
- [ ] 日志中显示 "Simulation collector initialized"
- [ ] 日志中显示仿真数据路径: `/data/sim/current.json`
- [ ] UAVMetrics资源被创建
- [ ] UAVMetrics包含所有新字段:
  - [ ] `spec.position.x/y/z`
  - [ ] `spec.velocity.vx/vy/vz`
  - [ ] `spec.simulation.vmId`
  - [ ] `spec.simulation.simulationId`
  - [ ] `spec.simulation.timeStep`
- [ ] GPS坐标正确映射（纬度39.9，经度116.4）
- [ ] 电池电量正确转换（96.7%）
- [ ] 每10秒自动更新数据

---

## 故障排查

### 如果Pod无法启动

1. **检查镜像**
```bash
kubectl describe pod <pod-name>
# 查看Events部分是否有镜像拉取错误
```

2. **检查仿真数据文件**
```bash
# 在每个节点上确认文件存在
ls -l /data/sim/current.json
cat /data/sim/current.json
```

3. **检查RBAC权限**
```bash
kubectl get clusterrole uav-agent -o yaml
kubectl get clusterrolebinding uav-agent -o yaml
```

### 如果UAVMetrics未创建

1. **查看Agent日志**
```bash
kubectl logs -l app=uav-agent-sim --tail=100
```

2. **检查API权限**
```bash
kubectl auth can-i create uavmetrics.uav.k3s.io --as=system:serviceaccount:default:uav-agent
```

3. **手动测试CRD创建**
```bash
cat <<EOF | kubectl apply -f -
apiVersion: uav.k3s.io/v1alpha1
kind: UAVMetrics
metadata:
  name: test-uav
  namespace: default
spec:
  nodeName: "test-node"
  gps:
    latitude: 39.9
    longitude: 116.4
  battery:
    remainingPercent: 95.0
EOF
```

---

## 总结

### ✅ 已就绪
- K3s集群: 12节点全部Ready
- UAVMetrics CRD: 已安装并包含新字段
- 仿真数据文件: 存在且格式正确
- 代码修改: 全部完成
- 配置文件: 已准备好

### ⚠️ 待处理
- 重新编译agent二进制（包含仿真代码）
- 构建新的Docker镜像
- 部署测试验证

### 🎯 预期结果
部署成功后，每个节点的agent将从 `/data/sim/current.json` 读取仿真数据，并每10秒更新一次UAVMetrics CRD，包含完整的位置、速度、GPS、电池等信息。

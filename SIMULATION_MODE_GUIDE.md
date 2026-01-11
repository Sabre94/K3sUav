# 仿真模式使用指南

## 概述

UAV Agent现在支持从仿真数据文件读取数据，无需真实硬件即可进行测试和开发。

## 修改内容

### 1. 数据模型扩展 (`pkg/models/types.go`)
新增以下数据结构：
- `PositionData`: 笛卡尔坐标系位置 (x, y, z)
- `VelocityData`: 速度向量 (vx, vy, vz)
- `SimulationInfo`: 仿真元数据 (vmId, simulationId, timeStep)

### 2. CRD定义更新 (`api/crd/uav-metrics-crd.yaml`)
在UAVMetrics CRD中添加了position、velocity、simulation字段，支持存储仿真数据。

### 3. 仿真数据采集器 (`pkg/collector/simulation_collector.go`)
新建SimulationCollector，从JSON文件读取仿真数据并转换为UAVMetrics格式。

支持的仿真数据格式：
```json
{
  "vm_id": "drone_000",
  "simulation_id": "drone_swarm_v1",
  "timestamp": "2024-01-01T00:06:35Z",
  "time_step": 79,
  "position": {
    "x": -3595.065997019947,
    "y": 4286.166292959703,
    "z": 10
  },
  "velocity": {
    "vx": -8.891917268879464,
    "vy": 12.080306588965495,
    "vz": 0
  },
  "geodetic": {
    "latitude": 39.94279458830275,
    "longitude": 116.36533565488828,
    "altitude": 62.45565287861973
  },
  "battery_level": 0.7111970611109146,
  "heading": 323.64443490413083,
  "altitude_agl": 10
}
```

### 4. 配置扩展 (`pkg/config/config.go`)
添加SimulationConfig配置项：
- `Enabled`: 是否启用仿真模式
- `DataPath`: 仿真数据文件路径

### 5. Agent主程序更新 (`cmd/agent/main.go`)
支持根据配置自动选择Real-time Collector或Simulation Collector。

## 使用方法

### 方法1: 环境变量配置（推荐）

启动agent时设置环境变量：

```bash
export SIMULATION_ENABLED=true
export SIMULATION_DATA_PATH=/data/sim/current.json
export NODE_NAME=drone-001

./bin/uav-agent
```

### 方法2: 在Kubernetes中部署

修改`deploy/agent-daemonset.yaml`，添加环境变量：

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: uav-agent
spec:
  template:
    spec:
      containers:
      - name: uav-agent
        image: your-registry/uav-agent:latest
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: SIMULATION_ENABLED
          value: "true"
        - name: SIMULATION_DATA_PATH
          value: "/data/sim/current.json"
        volumeMounts:
        - name: sim-data
          mountPath: /data/sim
          readOnly: true
      volumes:
      - name: sim-data
        hostPath:
          path: /data/sim
          type: Directory
```

## 数据字段映射

| 仿真数据字段 | UAVMetrics字段 | 说明 |
|------------|---------------|-----|
| geodetic.latitude | gps.latitude | GPS纬度 |
| geodetic.longitude | gps.longitude | GPS经度 |
| geodetic.altitude | gps.altitude | GPS海拔 |
| heading | gps.heading | 航向角 |
| battery_level | battery.remainingPercent | 电池电量(转换为百分比) |
| position | position | 笛卡尔坐标 |
| velocity | velocity | 速度向量 |
| vm_id | simulation.vmId | 仿真VM标识 |
| simulation_id | simulation.simulationId | 仿真场景ID |
| time_step | simulation.timeStep | 时间步 |
| altitude_agl | flight.altitude | 离地高度 |

## 验证

启动agent后，可以通过以下命令验证数据是否正确采集：

```bash
# 查看UAVMetrics CRD
kubectl get uavmetrics -o yaml

# 查看特定节点的数据
kubectl get uavmetrics <node-name> -o yaml

# 查看日志
kubectl logs -l app=uav-agent -f
```

## 注意事项

1. **文件更新**: SimulationCollector每次采集时重新读取JSON文件，支持实时更新仿真数据
2. **性能**: 对于高频更新，建议调整`COLLECTION_INTERVAL`环境变量（默认10s）
3. **错误处理**: 如果JSON文件不存在或格式错误，agent会记录错误但继续运行
4. **混合部署**: 可以在同一集群中混合部署仿真模式和真实模式的agent

## 示例：批量仿真节点

为多个仿真无人机创建数据文件：

```bash
# 创建仿真数据目录
mkdir -p /data/sim/drones

# 为每个无人机创建独立的数据文件
for i in {0..9}; do
  cp /data/sim/current.json /data/sim/drones/drone-00${i}.json
  # 根据需要修改每个文件的vm_id等字段
done
```

在DaemonSet中使用节点选择器和不同的挂载路径来实现多无人机仿真。

## 故障排查

### 问题1: agent无法读取仿真数据
- 检查文件路径是否正确
- 确认文件权限（agent需要读取权限）
- 查看agent日志中的错误信息

### 问题2: 数据未更新到CRD
- 检查Kubernetes RBAC权限
- 确认CRD已正确安装: `kubectl get crd uavmetrics.uav.k3s.io`
- 查看agent日志中的更新错误

### 问题3: JSON解析错误
- 验证JSON格式是否正确: `jq . /data/sim/current.json`
- 确认所有必需字段都存在
- 检查数值范围是否合法（如battery_level应在0-1之间）

## 后续扩展

可以进一步扩展的功能：
- 支持多种仿真数据格式（CSV, Protobuf等）
- 添加数据回放功能（按时间步播放历史数据）
- 支持动态仿真场景切换
- 集成仿真数据生成器

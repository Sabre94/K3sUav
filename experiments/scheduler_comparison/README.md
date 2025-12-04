# UAV Scheduler Comparison Experiments

## 实验环境说明

本目录包含 UAV 调度算法对比实验的基础环境。

### 当前文件

```
scheduler_comparison/
├── experiment_framework.go      # 实验框架（算法适配器、数据加载、覆盖率计算）
├── scripts/
│   └── generate_uav_data.py    # UAV 节点数据生成脚本
├── go.mod                       # Go 模块依赖
└── go.sum                       # Go 依赖校验和
```

### 使用说明

#### 1. 生成 UAV 数据

```bash
python3 scripts/generate_uav_data.py
```

这将生成：
- `data/uavmetrics.csv` - UAV 节点数据（GPS、电量、延迟、CPU、内存）
- `data/uavmetrics.yaml` - Kubernetes CRD 格式（如果安装了 yaml 模块）

配置参数（在脚本中修改）：
- `NODE_COUNT`: 节点数量（默认 26）
- `SPACING`: 相邻节点间距（默认 200m）
- `BASE_LAT/BASE_LON`: 编队中心坐标

#### 2. 实验框架

`experiment_framework.go` 提供了以下功能：

**算法适配器**：
- `greedNSGAIIAdapter` - GREED-NSGAII 算法适配器

**数据加载**：
- `LoadUAVMetrics()` - 从 CSV 加载 UAV 数据

**覆盖率计算**：
- `calculateCoverage()` - 使用网格采样法计算覆盖率

**类型定义**：
- `AlgorithmConfig` - 算法配置
- `ExperimentResult` - 实验结果

### 待创建

根据实验需求，需要创建：

1. **实验运行脚本**（例如 `run_experiments.go`）
   - 定义对比算法
   - 设置实验参数
   - 运行实验并收集结果

2. **可视化脚本**（例如 `visualize_results.py`）
   - 读取实验结果
   - 生成对比图表

3. **结果目录** `results/`
   - 存储实验数据（CSV）
   - 存储可视化图表（PNG）

### 依赖

**Go 依赖**（已在 go.mod 中定义）：
- `github.com/k3suav/uav-monitor` - UAV 监控和调度算法包

**Python 依赖**（可选）：
- `pandas` - 数据处理
- `matplotlib` - 可视化
- `seaborn` - 统计图表

### 算法说明

目前支持的调度算法：
1. **K8s-Default** - 基于资源利用率（Least-Loaded）
2. **Latency-First** - 基于网络延迟
3. **Workload-Aware** - 综合资源、延迟、电量
4. **GREED-NSGAII** - 基于覆盖率优化的两阶段混合算法

### 覆盖率计算方法

使用**网格采样法**（Grid Sampling）：
1. 将计算区域划分为 N×N 网格
2. 检查每个网格点是否在任意节点的覆盖圆内
3. 计算被覆盖的网格点占比
4. 覆盖率 = 选中节点覆盖面积 / 全部节点最大覆盖面积

详见：`experiment_framework.go:278` 的 `calculateCoverage()` 函数

---

**实验环境已就绪，等待新的实验任务！**

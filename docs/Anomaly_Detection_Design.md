# UAV 异常检测器设计文档

## 1. 概述

**异常检测器 (Anomaly Detector)** 是一个AI模块，用于检测无人机节点的异常状态，确保调度决策的安全性和可靠性。

### 1.1 检测目标

| 类别 | 异常类型 | 说明 |
|------|---------|------|
| 电池 | battery_drop | 电量骤降 |
| | battery_spike | 电量异常上升（虚电） |
| | battery_low | 电量过低 |
| | battery_critical | 电量危急 |
| 位置 | position_jump | 位置突变（GPS漂移） |
| | position_stuck | 位置卡住 |
| 网络 | latency_spike | 延迟突增 |
| | packet_loss | 丢包率高 |
| 性能 | cpu_high | CPU过高 |
| | memory_high | 内存过高 |
| | temperature_high | 温度过高 |

### 1.2 严重程度分级

| 级别 | 说明 | 处理动作 |
|------|------|---------|
| info | 信息 | 记录日志 |
| warning | 警告 | 发送告警 |
| critical | 严重 | 驱逐Pod |
| fatal | 致命 | 隔离节点 |

## 2. 模块架构

```
pkg/scheduler/anomaly/
├── types.go              # 数据类型定义
├── detector.go           # 主检测器入口
├── statistical.go        # 统计方法检测器 (Z-score)
├── isolation_forest.go   # Isolation Forest算法
└── rules.go              # 基于规则的检测器
```

## 3. 检测算法

### 3.1 统计方法检测器 (Z-score)

```
原理：
  Z-score = (x - μ) / σ

  μ = 滑动窗口均值
  σ = 滑动窗口标准差

判定：
  |Z-score| ≥ 3.0 → 异常
  |Z-score| ≥ 4.0 → 严重
  |Z-score| ≥ 5.0 → 致命
```

**特点**：
- 滑动窗口（默认30个数据点）
- 在线更新，无需批量计算
- 适合检测突变型异常

### 3.2 Isolation Forest

```
原理：
  - 异常点更容易被"隔离"
  - 构建多棵随机树
  - 计算样本的平均路径长度
  - 路径越短 → 越异常

异常分数：
  score = 2^(-avgPathLength / expectedPathLength)

  score ≥ 0.6 → 异常
  score ≥ 0.8 → 严重
  score ≥ 0.9 → 致命
```

**特点**：
- 无监督学习，不需要标注数据
- 可检测多维度异常
- 自动训练（收集256个样本后）

### 3.3 规则检测器

```
预定义规则：
  电池骤降: (lastBattery - currentBattery) / Δt > 5%/s
  低电量:   battery < 20%
  危急电量: battery < 10%
  位置突变: distance > 100m && speed > 30m/s
  延迟过高: latency > 500ms
  丢包过高: packetLoss > 10%
  CPU过高:  cpuUsage > 90%
  温度过高: temperature > 70°C
```

**特点**：
- 可解释性强
- 检测已知类型异常
- 可自定义阈值

## 4. 数据结构

```go
// 异常记录
type Anomaly struct {
    ID            string          // 异常ID
    NodeName      string          // 节点名称
    Type          AnomalyType     // 异常类型
    Severity      AnomalySeverity // 严重程度
    Score         float64         // 异常分数 (0-1)
    Message       string          // 异常描述
    DetectedAt    time.Time       // 检测时间
    DetectedBy    string          // 检测器名称
    CurrentValue  float64         // 当前值
    ExpectedValue float64         // 预期值
    Threshold     float64         // 阈值
}

// 节点异常状态
type NodeAnomalyState struct {
    NodeName        string
    IsHealthy       bool
    ActiveAnomalies []*Anomaly
    AnomalyScore    float64      // 综合异常分数
    HealthHistory   []bool       // 最近N次检查结果
}
```

## 5. 调度器集成

```go
// schedulePod 调度流程
func (s *Scheduler) schedulePod(ctx context.Context, pod *v1.Pod) error {
    // 1. 获取数据
    metrics, _ := s.uavClient.ListUAVMetrics(ctx)

    // 2. 状态预测（AI模块1）
    metrics = s.applyPredictions(metrics, predictedMetrics)

    // 3. 异常检测（AI模块2）
    anomalyResults := s.anomalyDetector.DetectBatch(metrics)

    // 4. 过滤不健康节点
    metrics = s.anomalyDetector.FilterHealthyMetrics(metrics)

    // 5. 执行调度算法
    scores, _ := selectedAlgo.Score(ctx, pod, metrics)
    // ...
}
```

## 6. 配置选项

```go
type DetectorConfig struct {
    // 启用的检测器
    EnableStatistical     bool  // 启用统计检测
    EnableIsolationForest bool  // 启用Isolation Forest
    EnableRuleBased       bool  // 启用规则检测

    // 统计检测器配置
    StatisticalWindowSize int     // 滑动窗口大小 (默认30)
    ZScoreThreshold       float64 // Z-score阈值 (默认3.0)

    // Isolation Forest配置
    IFNumTrees   int     // 树的数量 (默认100)
    IFSampleSize int     // 采样大小 (默认256)
    IFThreshold  float64 // 异常阈值 (默认0.6)

    // 规则检测器配置
    BatteryDropThreshold     float64 // 电量骤降阈值 (默认5%/s)
    BatteryLowThreshold      float64 // 低电量阈值 (默认20%)
    BatteryCriticalThreshold float64 // 危急电量阈值 (默认10%)
    PositionJumpThreshold    float64 // 位置突变阈值 (默认100m)
    LatencySpikeThreshold    float64 // 延迟突增阈值 (默认500ms)
    // ...
}
```

## 7. 检测流程

```
┌─────────────────────────────────────────────────────────────┐
│                    UAVMetrics 输入                          │
└─────────────────────────────────────────────────────────────┘
                             │
          ┌──────────────────┼──────────────────┐
          ▼                  ▼                  ▼
   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
   │ 规则检测器   │   │ 统计检测器   │   │ Isolation   │
   │ (Rules)     │   │ (Z-score)   │   │ Forest      │
   └─────────────┘   └─────────────┘   └─────────────┘
          │                  │                  │
          └──────────────────┼──────────────────┘
                             ▼
                    ┌─────────────────┐
                    │   去重 & 合并    │
                    └─────────────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │  更新节点状态    │
                    └─────────────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │  触发回调/告警   │
                    └─────────────────┘
```

## 8. 与调度器的协作

```
正常流程：
  Metrics → 预测增强 → 异常检测 → 过滤 → 调度算法 → 绑定

异常处理：
  检测到异常 → 记录日志 → 触发回调 → 根据严重程度决定动作

节点隔离：
  fatal异常 → 标记节点不健康 → 从调度候选中排除
```

## 9. 论文可写的点

| 创新点 | 描述 |
|--------|------|
| **混合检测架构** | 规则 + 统计 + ML三层检测 |
| **在线学习** | Isolation Forest自动训练 |
| **智能过滤** | 自动排除不健康节点 |
| **分级响应** | 根据严重程度采取不同动作 |
| **与预测器协同** | 预测 → 检测 → 调度闭环 |

## 10. 测试结果

```
检测场景          | 检测结果 | 严重程度
------------------|----------|----------
电量骤降 (50%)    | ✓ 检测到 | critical
低电量 (15%)      | ✓ 检测到 | warning
危急电量 (5%)     | ✓ 检测到 | fatal
位置突变 (500m)   | ✓ 检测到 | critical
延迟过高 (800ms)  | ✓ 检测到 | critical
高丢包率 (35%)    | ✓ 检测到 | critical
CPU过高 (95%)     | ✓ 检测到 | warning
温度过高 (85°C)   | ✓ 检测到 | critical
多重异常          | ✓ 全部检测 | 多级别
```

## 11. 代码统计

| 文件 | 行数 | 功能 |
|------|------|------|
| types.go | ~150 | 数据类型定义 |
| detector.go | ~300 | 主检测器入口 |
| statistical.go | ~180 | Z-score统计检测 |
| isolation_forest.go | ~280 | Isolation Forest |
| rules.go | ~250 | 规则检测器 |
| **总计** | **~1160** | - |

## 12. AI模块整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                   UAV 智能调度系统                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐       │
│  │ 状态预测器   │   │ 异常检测器   │   │ RL调度算法  │       │
│  │ (Predictor) │   │ (Anomaly)   │   │ (RL-Cov)   │       │
│  │             │   │             │   │             │       │
│  │ - LSTM      │   │ - Z-score   │   │ - Policy   │       │
│  │ - Kalman    │   │ - IForest   │   │   Gradient │       │
│  │ - EWMA      │   │ - Rules     │   │             │       │
│  └──────┬──────┘   └──────┬──────┘   └──────┬──────┘       │
│         │                 │                 │               │
│         └─────────────────┼─────────────────┘               │
│                           ▼                                 │
│                  ┌─────────────────┐                        │
│                  │    Scheduler    │                        │
│                  └─────────────────┘                        │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

现在系统具有完整的AI驱动调度能力：
1. **状态预测** - 解决数据陈旧问题
2. **异常检测** - 确保节点健康
3. **智能调度** - 优化资源分配

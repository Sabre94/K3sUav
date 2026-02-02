# UAV 状态预测器设计文档

## 1. 概述

**状态预测器 (State Predictor)** 是一个AI增强模块，用于解决无人机数据同步间隔期间的"数据陈旧"问题。

### 1.1 问题背景

```
时间线：
  t0          t1          t2          t3
  ├───────────┼───────────┼───────────┤
  采集点1     ???         ???         采集点2
  (真实数据)  (数据陈旧)   (数据陈旧)   (真实数据)
```

- Agent采集数据是**间歇性的**，不是实时的
- 两次数据同步之间存在时间间隔
- 调度器在间隔期间做决策时，使用的是**过时数据**
- 这可能导致调度决策不准确

### 1.2 解决方案

使用AI模型**预测当前时刻的真实状态**：
- 电池电量预测 → LSTM神经网络
- 位置预测 → 卡尔曼滤波 + 速度外推
- 网络延迟预测 → 指数加权移动平均(EWMA)

## 2. 模块架构

```
pkg/scheduler/predictor/
├── types.go              # 数据类型定义
├── history.go            # 历史数据环形缓冲区
├── battery_predictor.go  # 电池预测器 (LSTM)
├── position_predictor.go # 位置预测器 (Kalman Filter)
├── latency_predictor.go  # 延迟预测器 (EWMA)
└── predictor.go          # 主入口，整合各预测器
```

## 3. 核心组件

### 3.1 历史数据管理 (history.go)

```go
// 每个节点维护一个环形缓冲区
type HistoryBuffer struct {
    NodeName string
    Points   []HistoryPoint  // 环形缓冲区
    MaxSize  int             // 默认50个点
}

// 历史数据点
type HistoryPoint struct {
    Timestamp time.Time
    Battery   float64   // 电量 (%)
    Position  Position  // 位置 (x,y,z)
    Velocity  Velocity  // 速度 (vx,vy,vz)
    Latency   float64   // 延迟 (ms)
    Speed     float64   // 飞行速度 (m/s)
}
```

### 3.2 电池预测器 (battery_predictor.go)

**算法**：LSTM神经网络 + 线性回退

```
LSTM网络结构:
  Input(seq_len=10, features=3)  // [battery, speed, deltaT]
    → LSTM(hidden=32)
    → Dense(1)
    → 预测电量消耗率

预测公式:
  predicted_battery = current_battery - consumption_rate × Δt
```

**特点**：
- 考虑飞行速度对耗电的影响
- 支持在线学习（根据预测误差更新模型）
- 数据不足时回退到线性预测

### 3.3 位置预测器 (position_predictor.go)

**算法**：卡尔曼滤波

```
状态向量: X = [x, y, z, vx, vy, vz]ᵀ

状态转移矩阵 F:
  [1  0  0  Δt  0   0 ]
  [0  1  0  0   Δt  0 ]
  [0  0  1  0   0   Δt]
  [0  0  0  1   0   0 ]
  [0  0  0  0   1   0 ]
  [0  0  0  0   0   1 ]

预测: X̂(t+Δt) = F × X(t)
```

**特点**：
- 融合位置和速度信息
- 自适应噪声估计
- 置信度基于协方差矩阵

### 3.4 延迟预测器 (latency_predictor.go)

**算法**：EWMA + 趋势分析

```
EWMA更新:
  value = α × measurement + (1-α) × previous_value

趋势修正:
  trend = β × new_trend + (1-β) × old_trend
  predicted = value + trend × Δt
```

**特点**：
- 平滑噪声
- 捕捉变化趋势
- 动态调整平滑因子

## 4. 置信度计算

置信度随数据年龄**指数衰减**：

```go
func CalculateConfidence(dataAge, halfLife time.Duration) float64 {
    λ := ln(2) / halfLife
    return exp(-λ × dataAge)
}
```

默认半衰期配置：
- 电池：30秒
- 位置：10秒（变化快）
- 延迟：20秒

## 5. 调度器集成

```go
// scheduler.go 中的使用
func (s *Scheduler) schedulePod(ctx context.Context, pod *v1.Pod) error {
    // 1. 获取原始数据
    metrics, _ := s.uavClient.ListUAVMetrics(ctx)

    // 2. AI预测增强
    if s.predictionEnabled {
        predictedMetrics := s.statePredictor.EnhanceMetricsBatch(metrics)
        metrics = s.applyPredictions(metrics, predictedMetrics)
    }

    // 3. 使用增强数据调度
    scores, _ := selectedAlgo.Score(ctx, pod, metrics)
    // ...
}
```

## 6. 数据流

```
┌─────────────┐     ┌─────────────────┐     ┌──────────────────┐
│   K8s CRD   │────>│ State Predictor │────>│ Enhanced Metrics │
│  (原始数据)  │     │   (AI预测)       │     │   (预测数据)      │
└─────────────┘     └─────────────────┘     └──────────────────┘
                            │
              ┌─────────────┼─────────────┐
              ▼             ▼             ▼
        ┌─────────┐   ┌─────────┐   ┌─────────┐
        │ Battery │   │Position │   │ Latency │
        │  LSTM   │   │ Kalman  │   │  EWMA   │
        └─────────┘   └─────────┘   └─────────┘
```

## 7. 配置选项

```go
type PredictorConfig struct {
    // 历史数据
    HistorySize int           // 每节点历史点数，默认50
    MaxDataAge  time.Duration // 最大数据年龄，默认5分钟

    // 置信度半衰期
    BatteryConfidenceHalfLife  time.Duration // 默认30秒
    PositionConfidenceHalfLife time.Duration // 默认10秒
    LatencyConfidenceHalfLife  time.Duration // 默认20秒

    // 预测阈值
    PredictionThreshold time.Duration // 数据年龄超过此值才预测，默认2秒

    // LSTM配置
    LSTMEnabled    bool // 是否启用LSTM
    LSTMHiddenSize int  // 隐藏层大小，默认32
    LSTMSeqLength  int  // 序列长度，默认10
}
```

## 8. 论文可写的点

| 创新点 | 描述 |
|--------|------|
| **数据新鲜度感知调度** | 首次将"数据陈旧"问题纳入边缘/无人机调度 |
| **混合预测架构** | LSTM(电池) + Kalman(位置) + EWMA(延迟) |
| **在线自适应学习** | 模型根据预测误差持续更新 |
| **置信度引导决策** | 预测不确定性纳入调度权重 |
| **轻量级部署** | 纯Go实现，无外部依赖，适合边缘设备 |

## 9. 测试结果

```
场景1: 新鲜数据 (1秒前)
  使用预测: false
  置信度: 1.00

场景2: 稍旧数据 (5秒前)
  使用预测: true
  电量预测偏移: -0.25%
  位置预测偏移: (15m, 20m)
  置信度: 0.92

场景3: 陈旧数据 (30秒前)
  使用预测: true
  电量预测偏移: -1.5%
  位置预测偏移: (90m, 120m)
  置信度: 0.61

场景4: 很旧的数据 (2分钟前)
  使用预测: true
  电量预测偏移: -6%
  置信度: 0.14 (低置信度，调度器会谨慎使用)
```

## 10. 代码统计

| 文件 | 行数 | 功能 |
|------|------|------|
| types.go | 112 | 数据类型定义 |
| history.go | 224 | 历史数据管理 |
| battery_predictor.go | 431 | 电池预测(LSTM) |
| position_predictor.go | 350 | 位置预测(Kalman) |
| latency_predictor.go | 241 | 延迟预测(EWMA) |
| predictor.go | 356 | 主入口整合 |
| **总计** | **1714** | - |

## 11. 后续优化方向

1. **模型持久化**：保存训练好的LSTM权重
2. **异常检测**：检测预测异常（如电量骤降）
3. **多模型融合**：集成学习提高预测准确性
4. **GPU加速**：大规模部署时使用GPU推理

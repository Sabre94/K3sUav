# RL 覆盖率调度算法 - 训练指南

> 本文档帮助你理解 RL 训练的完整流程，应对答辩时老师可能的技术问题。

---

## 1. 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        训练流程                                  │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   ┌──────────────┐    ┌──────────────┐    ┌──────────────┐      │
│   │ DataGenerator│───►│  Trainer     │───►│ PolicyNetwork│      │
│   │  (生成数据)   │    │  (训练循环)   │    │  (策略网络)   │      │
│   └──────────────┘    └──────────────┘    └──────────────┘      │
│                              │                    ▲              │
│                              ▼                    │              │
│                       ┌──────────────┐            │              │
│                       │ Environment  │────────────┘              │
│                       │ (状态/奖励)   │                           │
│                       └──────────────┘                           │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. 核心组件详解

### 2.1 数据生成器 (DataGenerator)

**作用**：生成多样化的训练场景

**代码位置**：`data_generator.go`

```go
// 支持5种编队模式
EnableRandomPattern:  true   // 随机分布
EnableGridPattern:    true   // 网格编队
EnableLinePattern:    true   // 线性编队
EnableCirclePattern:  true   // 环形编队
EnableClusterPattern: true   // 聚类分布
```

**生成过程**：
1. 随机选择节点数量（如 15-60 个）
2. 随机选择区域大小（如 3000-15000 米）
3. 随机选择一种编队模式
4. 在该模式下生成节点坐标
5. 随机生成每个节点的属性（电量、延迟、CPU等）

**为什么这样设计**：
- 多样性 → 泛化能力
- 随机属性 → 不依赖特定数值
- 混合编队 → 一个模型处理所有情况

---

### 2.2 环境 (Environment)

**作用**：定义状态空间、动作空间、奖励函数

**代码位置**：`environment.go`

#### 2.2.1 状态空间 (State)

每个节点有 8 个特征，加上 4 个全局特征，共 **12 维输入**：

| 特征 | 维度 | 描述 | 归一化方式 |
|------|------|------|-----------|
| NormX | 1 | 归一化 X 坐标 | 0-1 |
| NormY | 1 | 归一化 Y 坐标 | 0-1 |
| Battery | 1 | 电量百分比 | 0-1 |
| Latency | 1 | 网络延迟 | 相对最大值 |
| CPUUsage | 1 | CPU 使用率 | 0-1 |
| MemoryUsage | 1 | 内存使用率 | 0-1 |
| DistanceToCenter | 1 | 到中心距离 | 相对区域大小 |
| NearestNeighborDist | 1 | 最近邻距离 | 相对覆盖半径 |
| CurrentCoverage | 1 | 当前覆盖率 | 0-1 |
| SelectedRatio | 1 | 已选节点比例 | 0-1 |
| TargetCoverage | 1 | 目标覆盖率 | 0-1 |
| SelectionMask | 1 | 是否可选 | 0或1 |

**为什么要归一化**：
- 不同特征量纲不同（延迟是毫秒，电量是百分比）
- 归一化后网络更容易学习
- 使模型对绝对数值不敏感，增强泛化

#### 2.2.2 动作空间 (Action)

- **动作定义**：选择一个未被选中的节点
- **动作空间大小**：等于当前未选节点数
- **终止条件**：覆盖率达到目标 或 所有节点已选

#### 2.2.3 奖励函数 (Reward) - 核心！

```
奖励 = 覆盖增量奖励 - 节点惩罚 + 效率奖励 + 达标奖励 + 空间分散奖励
```

**详细分解**：

| 奖励项 | 公式 | 作用 |
|--------|------|------|
| **覆盖增量** | `coverageGain × 10` | 鼓励选择高覆盖贡献节点 |
| **效率奖励** | 效率>1.5倍时 `+1.0` | 奖励高效节点 |
| **节点惩罚** | 基础 `-0.1`，接近目标时 `×4-10` | 防止选太多节点 |
| **使用率惩罚** | `nodeRatio × -0.5` | 使用越多惩罚越大 |
| **达标奖励** | `(1-nodeRatio) × 10` | 用更少节点达标奖励更高 |
| **空间分散** | 距离远 `+0.3`，太近 `-0.3` | 减少覆盖重叠 |

**设计理念**：

```
目标：用最少的节点达到目标覆盖率

实现方式：
1. 覆盖增量 → 选"有用"的节点
2. 节点惩罚 → 选"必要"的节点
3. 动态惩罚 → 越接近目标越谨慎
4. 达标奖励 → 节省节点有额外好处
```

---

### 2.3 策略网络 (PolicyNetwork)

**作用**：输入状态，输出每个节点的选择概率

**代码位置**：`model.go`

#### 2.3.1 网络结构

```
输入层 (12维)
    │
    ▼
隐藏层1 (128神经元, ReLU激活)
    │
    ▼
隐藏层2 (128神经元, ReLU激活)
    │
    ▼
输出层 (1维分数)
    │
    ▼
Softmax (转换为概率)
```

**关键代码**：

```go
// 前向传播
func (pn *PolicyNetwork) Forward(state *State) []float64 {
    scores := make([]float64, numNodes)

    for i := 0; i < numNodes; i++ {
        // 1. 构建输入: 节点特征 + 全局特征
        input := pn.buildInput(state, i)

        // 2. 通过隐藏层
        hidden := input
        for layer := 0; layer < numLayers; layer++ {
            hidden = denseLayer(hidden, weights[layer], biases[layer])
        }

        // 3. 输出分数
        scores[i] = mean(hidden)
    }

    // 4. 已选节点设为 -∞
    for i, mask := range state.SelectionMask {
        if mask == 0 {
            scores[i] = -1e9
        }
    }

    // 5. Softmax 转换为概率
    return softmax(scores)
}
```

#### 2.3.2 权重初始化

使用 **Xavier 初始化**：

```go
scale := math.Sqrt(2.0 / float64(inputSize + hiddenSize))
weight = rand.NormFloat64() * scale
```

**为什么用 Xavier**：
- 保持前向传播时方差稳定
- 避免梯度消失或爆炸
- 深度学习的标准做法

---

### 2.4 训练器 (Trainer)

**作用**：执行训练循环

**代码位置**：`trainer.go`

#### 2.4.1 训练流程

```
for episode = 0 to NumEpisodes:
    1. 随机选择一个训练场景
    2. 收集一个 Episode 的经验
    3. 计算梯度
    4. 更新网络权重
    5. (可选) 评估和保存模型
```

#### 2.4.2 经验收集 (collectEpisode)

```go
func collectEpisode(metrics) Episode {
    state = env.Reset(metrics)

    for step = 0 to MaxSteps:
        // 1. 策略网络选择动作 (带探索)
        action = policy.SelectAction(state, explore=true)

        // 2. 环境执行动作
        nextState, reward, done = env.Step(action)

        // 3. 记录经验
        experiences.append(state, action, reward, nextState)

        state = nextState
        if done: break

    return Episode{experiences, totalReward, finalCoverage}
}
```

**探索 vs 利用**：
- `explore=true`：按概率随机采样（训练时）
- `explore=false`：选概率最大的（推理时）

---

## 3. 策略梯度算法 (REINFORCE)

### 3.1 核心思想

> **好的动作 → 增加概率，坏的动作 → 减少概率**

数学形式：

```
∇θ J(θ) = E[ Σt Gt × ∇θ log π(at|st) ]
```

其中：
- `θ`：网络参数
- `Gt`：从 t 时刻开始的累积回报
- `π(at|st)`：在状态 st 下选择动作 at 的概率

### 3.2 回报计算

```go
func computeReturns(experiences, gamma=0.99) []float64 {
    returns := make([]float64, n)

    // 从后往前计算
    G := 0.0
    for t := n-1; t >= 0; t-- {
        G = experiences[t].Reward + gamma * G
        returns[t] = G
    }

    // 标准化（减均值除标准差）
    returns = normalize(returns)

    return returns
}
```

**为什么要折扣 (gamma=0.99)**：
- 近期奖励比远期奖励更重要
- 0.99 意味着：第 t 步的奖励对当前的贡献是 `0.99^t`

**为什么要标准化**：
- 减少梯度方差
- 加速收敛

### 3.3 梯度更新

```go
func UpdateWeights(gradients, learningRate) {
    for layer := 0; layer < numLayers; layer++ {
        for j := range weights[layer] {
            weights[layer][j] += learningRate * gradients[layer][j]
        }
    }
}
```

**梯度裁剪**：
```go
if gradient > 1.0 {
    gradient = 1.0
} else if gradient < -1.0 {
    gradient = -1.0
}
```

**为什么要裁剪**：防止梯度爆炸导致训练不稳定

---

## 4. 训练参数说明

| 参数 | 值 | 说明 |
|------|-----|------|
| NumEpisodes | 200-1000 | 训练轮数 |
| LearningRate | 0.005-0.01 | 学习率 |
| Gamma | 0.99 | 折扣因子 |
| HiddenSize | 128 | 隐藏层神经元数 |
| NumHiddenLayers | 2 | 隐藏层数量 |
| EpisodesPerUpdate | 5 | 每几个 episode 更新一次 |
| TargetCoverage | 0.85 | 训练时的目标覆盖率 |
| CoverageRadius | 500.0 | 覆盖半径（米） |
| GridDensity | 30 | 覆盖率计算网格密度 |

### 参数选择理由

- **HiddenSize=128**：问题不太复杂，128 够用
- **NumHiddenLayers=2**：两层足以学习非线性映射
- **LearningRate=0.005**：太大不稳定，太小收敛慢
- **Gamma=0.99**：标准值，平衡短期和长期奖励

---

## 5. 训练输出示例

```
============================================================
RL Coverage 模型训练
============================================================
训练集数量: 50
总 Episode: 200

Episode    50 | Reward:   3.42 | Coverage: 86.21% | Nodes: 18.3
Episode   100 | Reward:   4.87 | Coverage: 89.45% | Nodes: 16.8
Episode   150 | Reward:   5.23 | Coverage: 91.32% | Nodes: 15.2
Episode   200 | Reward:   5.61 | Coverage: 92.18% | Nodes: 14.5

训练完成! 耗时: 12.3s
```

**如何判断训练效果**：
- Reward 应该逐渐上升
- Coverage 应该稳定在目标值以上
- Nodes 应该逐渐下降（学会节省节点）

---

## 6. 推理流程

训练完成后，推理非常简单：

```go
func SelectNodes(metrics) ([]string, float64) {
    // 1. 重置环境
    state = env.Reset(metrics)

    // 2. 循环选择节点
    for step = 0 to maxSteps:
        // 达到目标就停止
        if coverage >= target:
            break

        // 选择最优动作（不探索）
        action = policy.SelectAction(state, explore=false)

        // 执行动作
        state, _, done = env.Step(action)

    // 3. 后处理：移除冗余节点
    selectedNodes = optimizeNodeSelection(selectedNodes)

    return selectedNodes, coverage
}
```

**推理优化**：
1. **提前终止**：达到目标覆盖率立即停止
2. **后处理剪枝**：尝试移除贡献最小的节点

---

## 7. 老师可能问的问题

### Q1: "为什么用 REINFORCE 而不是 DQN/PPO？"

**答**：
> "REINFORCE 是最简单的策略梯度算法，适合我们的问题：
> 1. 动作空间是离散的（选哪个节点）
> 2. Episode 长度短（几十步）
> 3. 问题规模不大
>
> 更复杂的算法如 PPO 需要更多工程，但收益有限。REINFORCE 已经达到了 100% 达标率。"

### Q2: "怎么保证收敛？"

**答**：
> "我们采取了几个措施：
> 1. **Xavier 初始化**：保持梯度稳定
> 2. **回报标准化**：减少方差
> 3. **梯度裁剪**：防止爆炸
> 4. **学习率调参**：0.005 是经过实验验证的
>
> 从训练日志看，Reward 和 Coverage 都是稳步上升的。"

### Q3: "训练多久？需要多少数据？"

**答**：
> "训练 200 个 episode，用 50 个训练场景，总耗时约 10-30 秒。
>
> 数据量不大的原因：
> 1. 问题规模小（节点数几十个）
> 2. 状态空间有限（12维特征）
> 3. 混合编队增加了多样性"

### Q4: "奖励函数怎么设计的？"

**答**：
> "奖励函数有5个组成部分（见上文详解），核心思想是：
> 1. 鼓励高覆盖增量（选有用的节点）
> 2. 惩罚节点数量（选必要的节点）
> 3. 动态调整（接近目标时更谨慎）
>
> 这种设计让网络学会'用最少节点达到目标覆盖率'。"

### Q5: "如果遇到训练时没见过的场景怎么办？"

**答**：
> "我们的特征都是归一化的、相对的，不依赖绝对数值。比如：
> - 坐标归一化到 0-1
> - 距离相对于覆盖半径
> - 已选比例相对于总节点数
>
> 这种设计让模型学到的是'规律'而不是'特定场景'。实验表明，训练 20-60 节点的模型，在 20/30/50 节点测试集上都能正常工作。"

---

## 8. 代码文件对照

| 文件 | 作用 | 核心函数 |
|------|------|---------|
| `algorithm.go` | 主入口 | `SelectNodes()`, `Train()` |
| `environment.go` | 环境 | `Reset()`, `Step()`, `calculateReward()` |
| `model.go` | 策略网络 | `Forward()`, `GetGradients()`, `UpdateWeights()` |
| `trainer.go` | 训练器 | `Train()`, `collectEpisode()` |
| `data_generator.go` | 数据生成 | `GenerateDiverseTrainingData()` |
| `types.go` | 数据结构 | `State`, `Action`, `Episode`, `RLConfig` |

---

## 9. 一张图总结训练过程

```
┌─────────────────────────────────────────────────────────────────┐
│                         训练循环                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────┐                                                     │
│  │ 数据生成 │ ─── 随机编队、随机节点数、随机属性                   │
│  └────┬────┘                                                     │
│       │                                                          │
│       ▼                                                          │
│  ┌─────────┐                                                     │
│  │ 环境重置 │ ─── 转换GPS坐标、计算特征、初始化覆盖率             │
│  └────┬────┘                                                     │
│       │                                                          │
│       ▼                                                          │
│  ┌─────────────────────────────────────┐                         │
│  │ 收集经验 (Episode)                   │                         │
│  │                                      │                         │
│  │   for each step:                     │                         │
│  │     action = 网络选择节点 (带随机探索) │                         │
│  │     reward = 环境返回奖励             │                         │
│  │     记录 (state, action, reward)     │                         │
│  │                                      │                         │
│  └────┬────────────────────────────────┘                         │
│       │                                                          │
│       ▼                                                          │
│  ┌─────────┐                                                     │
│  │ 计算回报 │ ─── G_t = r_t + 0.99*r_{t+1} + 0.99²*r_{t+2} + ...  │
│  └────┬────┘                                                     │
│       │                                                          │
│       ▼                                                          │
│  ┌─────────┐                                                     │
│  │ 计算梯度 │ ─── gradient = G_t × ∇log(π)                        │
│  └────┬────┘                                                     │
│       │                                                          │
│       ▼                                                          │
│  ┌─────────┐                                                     │
│  │ 更新权重 │ ─── weights += learning_rate × gradient            │
│  └────┬────┘                                                     │
│       │                                                          │
│       └─────────── 重复 200-1000 次 ───────────────────►完成      │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

*文档生成时间: 2024年*
*UAV覆盖率调度系统 - K3sUav Project*

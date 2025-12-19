package rl_coverage

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sync"
)

// PolicyNetwork 策略网络 (Actor)
// 输入: 节点特征 + 全局状态
// 输出: 每个节点的选择概率
type PolicyNetwork struct {
	// 网络参数
	inputSize  int
	hiddenSize int
	numLayers  int

	// 权重矩阵 (每层)
	weights [][]float64 // [layer][weight]
	biases  [][]float64 // [layer][bias]

	// 输出层 (动态大小，根据节点数)
	outputWeights []float64
	outputBias    float64

	mu sync.RWMutex
}

// NewPolicyNetwork 创建策略网络
func NewPolicyNetwork(inputSize, hiddenSize, numLayers int) *PolicyNetwork {
	pn := &PolicyNetwork{
		inputSize:  inputSize,
		hiddenSize: hiddenSize,
		numLayers:  numLayers,
		weights:    make([][]float64, numLayers),
		biases:     make([][]float64, numLayers),
	}

	// 初始化隐藏层
	for i := 0; i < numLayers; i++ {
		inSize := inputSize
		if i > 0 {
			inSize = hiddenSize
		}

		// Xavier 初始化
		scale := math.Sqrt(2.0 / float64(inSize+hiddenSize))

		pn.weights[i] = make([]float64, inSize*hiddenSize)
		pn.biases[i] = make([]float64, hiddenSize)

		for j := range pn.weights[i] {
			pn.weights[i][j] = rand.NormFloat64() * scale
		}
		for j := range pn.biases[i] {
			pn.biases[i][j] = 0.0
		}
	}

	return pn
}

// Forward 前向传播，返回每个节点的选择概率
func (pn *PolicyNetwork) Forward(state *State) []float64 {
	pn.mu.RLock()
	defer pn.mu.RUnlock()

	numNodes := len(state.NodeFeatures)
	if numNodes == 0 {
		return []float64{}
	}

	// 计算每个节点的分数
	scores := make([]float64, numNodes)

	for i := 0; i < numNodes; i++ {
		// 构建输入向量: 节点特征 + 全局特征
		input := pn.buildInput(state, i)

		// 通过隐藏层
		hidden := input
		for layer := 0; layer < pn.numLayers; layer++ {
			hidden = pn.denseLayer(hidden, pn.weights[layer], pn.biases[layer], pn.hiddenSize, true)
		}

		// 输出分数 (简化: 取隐藏层均值)
		score := 0.0
		for _, h := range hidden {
			score += h
		}
		scores[i] = score / float64(len(hidden))
	}

	// 应用选择掩码
	for i, mask := range state.SelectionMask {
		if mask == 0 {
			scores[i] = -1e9 // 已选择的节点设为极小值
		}
	}

	// Softmax 转换为概率
	probs := softmax(scores)

	return probs
}

// buildInput 构建输入向量
func (pn *PolicyNetwork) buildInput(state *State, nodeIdx int) []float64 {
	nf := state.NodeFeatures[nodeIdx]

	// 节点特征 (8个) + 全局特征 (4个) = 12 维输入
	input := []float64{
		// 节点特征
		nf.NormX,
		nf.NormY,
		nf.Battery,
		nf.Latency,
		nf.CPUUsage,
		nf.MemoryUsage,
		nf.DistanceToCenter,
		nf.NearestNeighborDist,

		// 全局特征
		state.CurrentCoverage,
		float64(state.SelectedCount) / float64(state.TotalNodes),
		state.TargetCoverage,
		state.SelectionMask[nodeIdx],
	}

	return input
}

// denseLayer 全连接层
func (pn *PolicyNetwork) denseLayer(input []float64, weights []float64, bias []float64, outSize int, useReLU bool) []float64 {
	inSize := len(input)
	output := make([]float64, outSize)

	for j := 0; j < outSize; j++ {
		sum := bias[j]
		for i := 0; i < inSize; i++ {
			sum += input[i] * weights[i*outSize+j]
		}

		if useReLU {
			output[j] = relu(sum)
		} else {
			output[j] = sum
		}
	}

	return output
}

// SelectAction 根据概率分布选择动作
func (pn *PolicyNetwork) SelectAction(state *State, explore bool) Action {
	probs := pn.Forward(state)

	if len(probs) == 0 {
		return Action{NodeIndex: -1, Prob: 1.0}
	}

	var selectedIdx int
	var selectedProb float64

	if explore {
		// 探索: 按概率采样
		r := rand.Float64()
		cumProb := 0.0
		for i, p := range probs {
			cumProb += p
			if r <= cumProb {
				selectedIdx = i
				selectedProb = p
				break
			}
		}
	} else {
		// 利用: 选择最大概率
		maxProb := -1.0
		for i, p := range probs {
			if p > maxProb {
				maxProb = p
				selectedIdx = i
				selectedProb = p
			}
		}
	}

	return Action{
		NodeIndex: selectedIdx,
		Prob:      selectedProb,
	}
}

// GetGradients 计算梯度 (Policy Gradient with REINFORCE)
// 使用简化的解析梯度计算，避免数值梯度的高计算成本
func (pn *PolicyNetwork) GetGradients(episodes []Episode) [][][]float64 {
	gradients := make([][][]float64, pn.numLayers)
	for i := 0; i < pn.numLayers; i++ {
		gradients[i] = make([][]float64, 2) // [weights, biases]
		gradients[i][0] = make([]float64, len(pn.weights[i]))
		gradients[i][1] = make([]float64, len(pn.biases[i]))
	}

	for _, ep := range episodes {
		// 计算回报 G_t
		returns := computeReturns(ep.Experiences, 0.99)

		for t, exp := range ep.Experiences {
			if exp.Action.NodeIndex < 0 || exp.Action.NodeIndex >= len(exp.State.NodeFeatures) {
				continue
			}

			advantage := returns[t]

			// 使用简化的梯度估计: 增强被选择动作的特征响应
			// 对于策略梯度，我们需要增加选中动作的概率
			input := pn.buildInput(exp.State, exp.Action.NodeIndex)

			// 计算梯度贡献: 对于被选择的动作，梯度正比于 advantage * input
			// 这是一个简化的近似，但对于demo足够
			for layer := 0; layer < pn.numLayers; layer++ {
				inSize := pn.inputSize
				if layer > 0 {
					inSize = pn.hiddenSize
				}

				// 简化: 只更新与输入相关的权重
				scale := advantage * 0.01 // 缩放因子
				for j := 0; j < pn.hiddenSize; j++ {
					for i := 0; i < inSize && i < len(input); i++ {
						idx := i*pn.hiddenSize + j
						if idx < len(gradients[layer][0]) {
							gradients[layer][0][idx] += scale * input[i]
						}
					}
					if j < len(gradients[layer][1]) {
						gradients[layer][1][j] += scale
					}
				}

				// 为下一层准备: 通过当前层前向传播
				if layer < pn.numLayers-1 {
					input = pn.denseLayer(input, pn.weights[layer], pn.biases[layer], pn.hiddenSize, true)
				}
			}
		}
	}

	// 平均化并裁剪梯度
	numSamples := float64(len(episodes))
	if numSamples == 0 {
		numSamples = 1
	}
	for layer := 0; layer < pn.numLayers; layer++ {
		for j := range gradients[layer][0] {
			gradients[layer][0][j] /= numSamples
			// 梯度裁剪
			if gradients[layer][0][j] > 1.0 {
				gradients[layer][0][j] = 1.0
			} else if gradients[layer][0][j] < -1.0 {
				gradients[layer][0][j] = -1.0
			}
		}
		for j := range gradients[layer][1] {
			gradients[layer][1][j] /= numSamples
			if gradients[layer][1][j] > 1.0 {
				gradients[layer][1][j] = 1.0
			} else if gradients[layer][1][j] < -1.0 {
				gradients[layer][1][j] = -1.0
			}
		}
	}

	return gradients
}

// numericalGradient 数值梯度计算
func (pn *PolicyNetwork) numericalGradient(state *State, actionIdx int, layer, paramIdx int, isWeight bool) float64 {
	eps := 1e-4

	// 保存原值
	var original float64
	if isWeight {
		original = pn.weights[layer][paramIdx]
	} else {
		original = pn.biases[layer][paramIdx]
	}

	// 计算 f(x + eps)
	if isWeight {
		pn.weights[layer][paramIdx] = original + eps
	} else {
		pn.biases[layer][paramIdx] = original + eps
	}
	probs1 := pn.Forward(state)
	logProb1 := math.Log(probs1[actionIdx] + 1e-10)

	// 计算 f(x - eps)
	if isWeight {
		pn.weights[layer][paramIdx] = original - eps
	} else {
		pn.biases[layer][paramIdx] = original - eps
	}
	probs2 := pn.Forward(state)
	logProb2 := math.Log(probs2[actionIdx] + 1e-10)

	// 恢复原值
	if isWeight {
		pn.weights[layer][paramIdx] = original
	} else {
		pn.biases[layer][paramIdx] = original
	}

	// 数值梯度
	return (logProb1 - logProb2) / (2 * eps)
}

// UpdateWeights 更新权重
func (pn *PolicyNetwork) UpdateWeights(gradients [][][]float64, learningRate float64) {
	pn.mu.Lock()
	defer pn.mu.Unlock()

	for layer := 0; layer < pn.numLayers; layer++ {
		for j := range pn.weights[layer] {
			pn.weights[layer][j] += learningRate * gradients[layer][0][j]
		}
		for j := range pn.biases[layer] {
			pn.biases[layer][j] += learningRate * gradients[layer][1][j]
		}
	}
}

// Save 保存模型
func (pn *PolicyNetwork) Save(filepath string) error {
	pn.mu.RLock()
	defer pn.mu.RUnlock()

	data := map[string]interface{}{
		"input_size":  pn.inputSize,
		"hidden_size": pn.hiddenSize,
		"num_layers":  pn.numLayers,
		"weights":     pn.weights,
		"biases":      pn.biases,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath, jsonData, 0644)
}

// Load 加载模型
func (pn *PolicyNetwork) Load(filepath string) error {
	pn.mu.Lock()
	defer pn.mu.Unlock()

	jsonData, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return err
	}

	pn.inputSize = int(data["input_size"].(float64))
	pn.hiddenSize = int(data["hidden_size"].(float64))
	pn.numLayers = int(data["num_layers"].(float64))

	// 解析权重
	weightsData := data["weights"].([]interface{})
	pn.weights = make([][]float64, len(weightsData))
	for i, w := range weightsData {
		wArr := w.([]interface{})
		pn.weights[i] = make([]float64, len(wArr))
		for j, v := range wArr {
			pn.weights[i][j] = v.(float64)
		}
	}

	// 解析偏置
	biasesData := data["biases"].([]interface{})
	pn.biases = make([][]float64, len(biasesData))
	for i, b := range biasesData {
		bArr := b.([]interface{})
		pn.biases[i] = make([]float64, len(bArr))
		for j, v := range bArr {
			pn.biases[i][j] = v.(float64)
		}
	}

	return nil
}

// 辅助函数

func relu(x float64) float64 {
	if x > 0 {
		return x
	}
	return 0
}

func softmax(scores []float64) []float64 {
	if len(scores) == 0 {
		return []float64{}
	}

	// 数值稳定性: 减去最大值
	maxScore := scores[0]
	for _, s := range scores {
		if s > maxScore {
			maxScore = s
		}
	}

	expSum := 0.0
	probs := make([]float64, len(scores))
	for i, s := range scores {
		probs[i] = math.Exp(s - maxScore)
		expSum += probs[i]
	}

	for i := range probs {
		probs[i] /= expSum
	}

	return probs
}

func computeReturns(experiences []Experience, gamma float64) []float64 {
	n := len(experiences)
	returns := make([]float64, n)

	// 从后往前计算累积回报
	G := 0.0
	for t := n - 1; t >= 0; t-- {
		G = experiences[t].Reward + gamma*G
		returns[t] = G
	}

	// 标准化
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(n)

	std := 0.0
	for _, r := range returns {
		std += (r - mean) * (r - mean)
	}
	std = math.Sqrt(std/float64(n) + 1e-8)

	for i := range returns {
		returns[i] = (returns[i] - mean) / std
	}

	return returns
}

// String 返回网络描述
func (pn *PolicyNetwork) String() string {
	return fmt.Sprintf("PolicyNetwork(input=%d, hidden=%d, layers=%d)",
		pn.inputSize, pn.hiddenSize, pn.numLayers)
}

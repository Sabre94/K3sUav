package greed_nsgaii

import (
	"math"
	"math/rand"
	"sort"
)

// NSGA2Config NSGA-II 配置参数
type NSGA2Config struct {
	PopulationSize   int     // 种群大小
	Generations      int     // 迭代代数
	CrossoverRate    float64 // 交叉概率
	MutationRate     float64 // 变异概率
	CoverageConfig   *CoverageConfig
	GreedySelector   *GreedySelector
}

// NSGA2Optimizer NSGA-II 优化器
type NSGA2Optimizer struct {
	config    *NSGA2Config
	allNodes  []*NodeInfo
	plotArea  PlotArea
	maxArea   float64
	rng       *rand.Rand
}

// NewNSGA2Optimizer 创建 NSGA-II 优化器
func NewNSGA2Optimizer(config *NSGA2Config, allNodes []*NodeInfo) *NSGA2Optimizer {
	plotArea := CalculatePlotArea(allNodes, config.CoverageConfig.CoverageRadius)
	maxArea := CalculateUnionArea(allNodes, plotArea, config.CoverageConfig.CoverageRadius, config.CoverageConfig.GridDensity)

	return &NSGA2Optimizer{
		config:   config,
		allNodes: allNodes,
		plotArea: plotArea,
		maxArea:  maxArea,
		rng:      rand.New(rand.NewSource(42)),
	}
}

// Optimize 执行 NSGA-II 优化
func (opt *NSGA2Optimizer) Optimize() *NSGA2Result {
	// 1. 初始化种群
	population := opt.initializePopulation()

	// 2. 评估初始种群
	opt.evaluatePopulation(population)

	// 3. 迭代进化
	for gen := 0; gen < opt.config.Generations; gen++ {
		// 生成子代
		offspring := opt.generateOffspring(population)

		// 评估子代
		opt.evaluatePopulation(offspring)

		// 合并父代和子代
		combined := append(population, offspring...)

		// 非支配排序
		opt.fastNonDominatedSort(combined)

		// 选择下一代
		population = opt.selectNextGeneration(combined)
	}

	// 4. 提取 Pareto 前沿
	paretoFront := opt.extractParetoFront(population)

	// 5. 选择推荐解（使用膝点法或折中解）
	bestSolution := opt.selectBestSolution(paretoFront)

	return &NSGA2Result{
		ParetoFront:   paretoFront,
		BestSolution:  bestSolution,
		AllPopulation: population,
	}
}

// initializePopulation 初始化种群
func (opt *NSGA2Optimizer) initializePopulation() Population {
	population := make(Population, opt.config.PopulationSize)

	for i := 0; i < opt.config.PopulationSize; i++ {
		// 随机生成染色体
		chromosome := make([]bool, len(opt.allNodes))
		for j := range chromosome {
			chromosome[j] = opt.rng.Float64() < 0.5
		}

		// 修复染色体（确保满足覆盖率约束）
		chromosome = opt.repairChromosome(chromosome)

		population[i] = &Individual{
			Chromosome: chromosome,
		}
	}

	return population
}

// evaluatePopulation 评估种群
func (opt *NSGA2Optimizer) evaluatePopulation(population Population) {
	for _, individual := range population {
		opt.evaluateIndividual(individual)
	}
}

// evaluateIndividual 评估个体
func (opt *NSGA2Optimizer) evaluateIndividual(individual *Individual) {
	// 1. 提取选中的节点
	selectedNodes := opt.getSelectedNodes(individual.Chromosome)

	// 2. 计算目标值
	individual.Objectives = CalculateMultiObjectives(selectedNodes)

	// 3. 检查约束（覆盖率）
	individual.IsFeasible = opt.checkConstraint(selectedNodes)
}

// getSelectedNodes 根据染色体提取选中的节点
func (opt *NSGA2Optimizer) getSelectedNodes(chromosome []bool) []*NodeInfo {
	selectedNodes := []*NodeInfo{}
	for i, selected := range chromosome {
		if selected {
			selectedNodes = append(selectedNodes, opt.allNodes[i])
		}
	}
	return selectedNodes
}

// checkConstraint 检查覆盖率约束
func (opt *NSGA2Optimizer) checkConstraint(selectedNodes []*NodeInfo) bool {
	if len(selectedNodes) == 0 {
		return false
	}

	currentArea := CalculateUnionArea(selectedNodes, opt.plotArea, opt.config.CoverageConfig.CoverageRadius, opt.config.CoverageConfig.GridDensity)
	coverageRatio := CalculateCoverageRatio(currentArea, opt.maxArea)

	return coverageRatio >= opt.config.CoverageConfig.TargetCoverageRatio
}

// generateOffspring 生成子代
func (opt *NSGA2Optimizer) generateOffspring(population Population) Population {
	offspring := make(Population, 0, opt.config.PopulationSize)

	for len(offspring) < opt.config.PopulationSize {
		// 锦标赛选择两个父代
		parent1 := opt.tournamentSelection(population)
		parent2 := opt.tournamentSelection(population)

		// 交叉
		child1, child2 := opt.crossover(parent1, parent2)

		// 变异
		opt.mutate(child1)
		opt.mutate(child2)

		// 修复
		child1.Chromosome = opt.repairChromosome(child1.Chromosome)
		child2.Chromosome = opt.repairChromosome(child2.Chromosome)

		offspring = append(offspring, child1, child2)
	}

	// 截断到指定大小
	if len(offspring) > opt.config.PopulationSize {
		offspring = offspring[:opt.config.PopulationSize]
	}

	return offspring
}

// tournamentSelection 锦标赛选择
func (opt *NSGA2Optimizer) tournamentSelection(population Population) *Individual {
	tournamentSize := 2
	best := population[opt.rng.Intn(len(population))]

	for i := 1; i < tournamentSize; i++ {
		candidate := population[opt.rng.Intn(len(population))]
		if CrowdingDistanceComparison(candidate, best) {
			best = candidate
		}
	}

	return best
}

// crossover 交叉操作（单点交叉）
func (opt *NSGA2Optimizer) crossover(parent1, parent2 *Individual) (*Individual, *Individual) {
	child1 := &Individual{Chromosome: make([]bool, len(parent1.Chromosome))}
	child2 := &Individual{Chromosome: make([]bool, len(parent2.Chromosome))}

	if opt.rng.Float64() < opt.config.CrossoverRate {
		// 单点交叉
		crossoverPoint := opt.rng.Intn(len(parent1.Chromosome))

		copy(child1.Chromosome[:crossoverPoint], parent1.Chromosome[:crossoverPoint])
		copy(child1.Chromosome[crossoverPoint:], parent2.Chromosome[crossoverPoint:])

		copy(child2.Chromosome[:crossoverPoint], parent2.Chromosome[:crossoverPoint])
		copy(child2.Chromosome[crossoverPoint:], parent1.Chromosome[crossoverPoint:])
	} else {
		// 不交叉，直接复制
		copy(child1.Chromosome, parent1.Chromosome)
		copy(child2.Chromosome, parent2.Chromosome)
	}

	return child1, child2
}

// mutate 变异操作（位翻转）
func (opt *NSGA2Optimizer) mutate(individual *Individual) {
	for i := range individual.Chromosome {
		if opt.rng.Float64() < opt.config.MutationRate {
			individual.Chromosome[i] = !individual.Chromosome[i]
		}
	}
}

// repairChromosome 修复染色体（使用贪心算法确保满足覆盖率约束）
func (opt *NSGA2Optimizer) repairChromosome(chromosome []bool) []bool {
	selectedNodes := opt.getSelectedNodes(chromosome)

	// 检查是否已满足约束
	if opt.checkConstraint(selectedNodes) {
		return chromosome
	}

	// 如果不满足约束，使用贪心算法添加节点
	repairedChromosome := make([]bool, len(chromosome))
	copy(repairedChromosome, chromosome)

	// 获取未选择的节点
	unselectedIndices := []int{}
	for i, selected := range repairedChromosome {
		if !selected {
			unselectedIndices = append(unselectedIndices, i)
		}
	}

	// 贪心添加节点直到满足约束
	for len(unselectedIndices) > 0 {
		selectedNodes = opt.getSelectedNodes(repairedChromosome)
		if opt.checkConstraint(selectedNodes) {
			break
		}

		// 找出增益最大的未选择节点
		bestIdx := -1
		bestGain := -1.0

		for _, idx := range unselectedIndices {
			node := opt.allNodes[idx]
			incrementalArea := CalculateIncrementalArea(node, selectedNodes, opt.plotArea, opt.config.CoverageConfig.CoverageRadius, opt.config.CoverageConfig.GridDensity)
			gain := incrementalArea * node.Score

			if gain > bestGain {
				bestGain = gain
				bestIdx = idx
			}
		}

		if bestIdx == -1 || bestGain <= 0 {
			break
		}

		// 选择该节点
		repairedChromosome[bestIdx] = true

		// 从未选择列表中移除
		for i, idx := range unselectedIndices {
			if idx == bestIdx {
				unselectedIndices = append(unselectedIndices[:i], unselectedIndices[i+1:]...)
				break
			}
		}
	}

	return repairedChromosome
}

// fastNonDominatedSort 快速非支配排序
func (opt *NSGA2Optimizer) fastNonDominatedSort(population Population) {
	// 初始化
	dominatedCount := make([]int, len(population))
	dominatingSet := make([][]int, len(population))

	fronts := [][]int{}
	fronts = append(fronts, []int{})

	// 计算支配关系
	for i := 0; i < len(population); i++ {
		dominatingSet[i] = []int{}
		dominatedCount[i] = 0

		for j := 0; j < len(population); j++ {
			if i == j {
				continue
			}

			if Dominates(population[i], population[j]) {
				// i 支配 j
				dominatingSet[i] = append(dominatingSet[i], j)
			} else if Dominates(population[j], population[i]) {
				// j 支配 i
				dominatedCount[i]++
			}
		}

		// 如果 i 没有被任何个体支配，加入第一前沿
		if dominatedCount[i] == 0 {
			population[i].Rank = 1
			fronts[0] = append(fronts[0], i)
		}
	}

	// 构建后续前沿
	currentRank := 1
	for len(fronts[currentRank-1]) > 0 {
		nextFront := []int{}

		for _, i := range fronts[currentRank-1] {
			for _, j := range dominatingSet[i] {
				dominatedCount[j]--
				if dominatedCount[j] == 0 {
					population[j].Rank = currentRank + 1
					nextFront = append(nextFront, j)
				}
			}
		}

		if len(nextFront) > 0 {
			fronts = append(fronts, nextFront)
			currentRank++
		} else {
			break
		}
	}

	// 为每个前沿计算拥挤度距离
	for _, front := range fronts {
		opt.calculateCrowdingDistance(front, population)
	}
}

// calculateCrowdingDistance 计算拥挤度距离
func (opt *NSGA2Optimizer) calculateCrowdingDistance(frontIndices []int, population Population) {
	if len(frontIndices) == 0 {
		return
	}

	// 初始化拥挤度为 0
	for _, idx := range frontIndices {
		population[idx].CrowdingDistance = 0
	}

	// 如果前沿只有 1-2 个个体，设置为无穷大
	if len(frontIndices) <= 2 {
		for _, idx := range frontIndices {
			population[idx].CrowdingDistance = math.Inf(1)
		}
		return
	}

	// 对每个目标维度计算拥挤度
	numObjectives := len(population[frontIndices[0]].Objectives)

	for objIdx := 0; objIdx < numObjectives; objIdx++ {
		// 按该目标维度排序
		sortedIndices := make([]int, len(frontIndices))
		copy(sortedIndices, frontIndices)

		sort.Slice(sortedIndices, func(i, j int) bool {
			return population[sortedIndices[i]].Objectives[objIdx] < population[sortedIndices[j]].Objectives[objIdx]
		})

		// 边界个体设置为无穷大
		population[sortedIndices[0]].CrowdingDistance = math.Inf(1)
		population[sortedIndices[len(sortedIndices)-1]].CrowdingDistance = math.Inf(1)

		// 计算目标范围
		objMin := population[sortedIndices[0]].Objectives[objIdx]
		objMax := population[sortedIndices[len(sortedIndices)-1]].Objectives[objIdx]
		objRange := objMax - objMin

		if objRange == 0 {
			continue
		}

		// 计算中间个体的拥挤度
		for i := 1; i < len(sortedIndices)-1; i++ {
			if !math.IsInf(population[sortedIndices[i]].CrowdingDistance, 1) {
				distance := (population[sortedIndices[i+1]].Objectives[objIdx] - population[sortedIndices[i-1]].Objectives[objIdx]) / objRange
				population[sortedIndices[i]].CrowdingDistance += distance
			}
		}
	}
}

// selectNextGeneration 选择下一代
func (opt *NSGA2Optimizer) selectNextGeneration(combined Population) Population {
	// 已经排序好，直接选择前 N 个
	nextGen := make(Population, 0, opt.config.PopulationSize)

	// 按 rank 和拥挤度排序
	sort.Slice(combined, func(i, j int) bool {
		return CrowdingDistanceComparison(combined[i], combined[j])
	})

	// 选择前 N 个
	for i := 0; i < opt.config.PopulationSize && i < len(combined); i++ {
		nextGen = append(nextGen, combined[i])
	}

	return nextGen
}

// extractParetoFront 提取 Pareto 前沿（rank = 1）
func (opt *NSGA2Optimizer) extractParetoFront(population Population) Population {
	paretoFront := Population{}
	for _, individual := range population {
		if individual.Rank == 1 {
			paretoFront = append(paretoFront, individual)
		}
	}
	return paretoFront
}

// selectBestSolution 选择推荐解（使用膝点法）
func (opt *NSGA2Optimizer) selectBestSolution(paretoFront Population) *Individual {
	if len(paretoFront) == 0 {
		return nil
	}

	// 简化策略：选择拥挤度最大的解（最具代表性）
	best := paretoFront[0]
	for _, individual := range paretoFront {
		if individual.CrowdingDistance > best.CrowdingDistance {
			best = individual
		}
	}

	return best
}

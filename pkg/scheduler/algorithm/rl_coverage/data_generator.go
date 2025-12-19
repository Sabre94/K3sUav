package rl_coverage

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/k3suav/uav-monitor/pkg/models"
)

// DataGenerator 多样化训练数据生成器
type DataGenerator struct {
	config *GeneratorConfig
}

// GeneratorConfig 数据生成配置
type GeneratorConfig struct {
	// 节点数量范围 (覆盖各种编队规模)
	MinNodes int // 最小 5
	MaxNodes int // 最大 200

	// 区域大小范围 (米)
	MinAreaSize float64 // 最小区域 500米
	MaxAreaSize float64 // 最大区域 50000米 (50公里)

	// 编队模式
	EnableRandomPattern  bool // 随机分布
	EnableGridPattern    bool // 网格分布
	EnableLinePattern    bool // 线性编队
	EnableCirclePattern  bool // 环形编队
	EnableClusterPattern bool // 聚类分布
}

// DefaultGeneratorConfig 默认配置 - 涵盖各种情况
func DefaultGeneratorConfig() *GeneratorConfig {
	return &GeneratorConfig{
		MinNodes:             5,
		MaxNodes:             200,
		MinAreaSize:          500,
		MaxAreaSize:          50000,
		EnableRandomPattern:  true,
		EnableGridPattern:    true,
		EnableLinePattern:    true,
		EnableCirclePattern:  true,
		EnableClusterPattern: true,
	}
}

// NewDataGenerator 创建数据生成器
func NewDataGenerator(config *GeneratorConfig) *DataGenerator {
	if config == nil {
		config = DefaultGeneratorConfig()
	}
	return &DataGenerator{config: config}
}

// GenerateDiverseTrainingData 生成多样化训练数据
func (g *DataGenerator) GenerateDiverseTrainingData(numSets int) [][]*models.UAVMetrics {
	rand.Seed(time.Now().UnixNano())

	data := make([][]*models.UAVMetrics, numSets)
	patterns := g.getEnabledPatterns()

	for i := 0; i < numSets; i++ {
		// 随机节点数量
		numNodes := g.config.MinNodes + rand.Intn(g.config.MaxNodes-g.config.MinNodes+1)

		// 随机区域大小
		areaSize := g.config.MinAreaSize + rand.Float64()*(g.config.MaxAreaSize-g.config.MinAreaSize)

		// 随机选择编队模式
		pattern := patterns[rand.Intn(len(patterns))]

		data[i] = g.generateByPattern(numNodes, areaSize, pattern)
	}

	return data
}

// 获取启用的模式
func (g *DataGenerator) getEnabledPatterns() []string {
	patterns := []string{}
	if g.config.EnableRandomPattern {
		patterns = append(patterns, "random")
	}
	if g.config.EnableGridPattern {
		patterns = append(patterns, "grid")
	}
	if g.config.EnableLinePattern {
		patterns = append(patterns, "line")
	}
	if g.config.EnableCirclePattern {
		patterns = append(patterns, "circle")
	}
	if g.config.EnableClusterPattern {
		patterns = append(patterns, "cluster")
	}
	if len(patterns) == 0 {
		patterns = []string{"random"}
	}
	return patterns
}

// 根据模式生成数据
func (g *DataGenerator) generateByPattern(numNodes int, areaSize float64, pattern string) []*models.UAVMetrics {
	// 基准坐标 (随机选择全球任意位置)
	baseLat := -60.0 + rand.Float64()*120.0 // -60 到 60 度
	baseLon := -180.0 + rand.Float64()*360.0

	// 将区域大小转换为经纬度偏移 (近似)
	latRange := areaSize / 111000.0 // 1度约111公里
	lonRange := areaSize / (111000.0 * math.Cos(baseLat*math.Pi/180.0))

	metrics := make([]*models.UAVMetrics, numNodes)

	switch pattern {
	case "grid":
		metrics = g.generateGrid(numNodes, baseLat, baseLon, latRange, lonRange)
	case "line":
		metrics = g.generateLine(numNodes, baseLat, baseLon, latRange, lonRange)
	case "circle":
		metrics = g.generateCircle(numNodes, baseLat, baseLon, latRange, lonRange)
	case "cluster":
		metrics = g.generateCluster(numNodes, baseLat, baseLon, latRange, lonRange)
	default:
		metrics = g.generateRandom(numNodes, baseLat, baseLon, latRange, lonRange)
	}

	return metrics
}

// 随机分布
func (g *DataGenerator) generateRandom(numNodes int, baseLat, baseLon, latRange, lonRange float64) []*models.UAVMetrics {
	metrics := make([]*models.UAVMetrics, numNodes)
	for i := 0; i < numNodes; i++ {
		metrics[i] = g.createNode(i,
			baseLat+rand.Float64()*latRange,
			baseLon+rand.Float64()*lonRange)
	}
	return metrics
}

// 网格分布
func (g *DataGenerator) generateGrid(numNodes int, baseLat, baseLon, latRange, lonRange float64) []*models.UAVMetrics {
	metrics := make([]*models.UAVMetrics, numNodes)
	gridSize := int(math.Ceil(math.Sqrt(float64(numNodes))))

	for i := 0; i < numNodes; i++ {
		row := i / gridSize
		col := i % gridSize
		lat := baseLat + float64(row)/float64(gridSize)*latRange
		lon := baseLon + float64(col)/float64(gridSize)*lonRange
		// 添加小量随机噪声
		lat += (rand.Float64() - 0.5) * latRange * 0.1 / float64(gridSize)
		lon += (rand.Float64() - 0.5) * lonRange * 0.1 / float64(gridSize)
		metrics[i] = g.createNode(i, lat, lon)
	}
	return metrics
}

// 线性编队
func (g *DataGenerator) generateLine(numNodes int, baseLat, baseLon, latRange, lonRange float64) []*models.UAVMetrics {
	metrics := make([]*models.UAVMetrics, numNodes)
	angle := rand.Float64() * math.Pi // 随机角度

	for i := 0; i < numNodes; i++ {
		t := float64(i) / float64(numNodes-1)
		lat := baseLat + t*latRange*math.Sin(angle)
		lon := baseLon + t*lonRange*math.Cos(angle)
		// 添加小量随机噪声
		lat += (rand.Float64() - 0.5) * latRange * 0.05
		lon += (rand.Float64() - 0.5) * lonRange * 0.05
		metrics[i] = g.createNode(i, lat, lon)
	}
	return metrics
}

// 环形编队
func (g *DataGenerator) generateCircle(numNodes int, baseLat, baseLon, latRange, lonRange float64) []*models.UAVMetrics {
	metrics := make([]*models.UAVMetrics, numNodes)
	centerLat := baseLat + latRange/2
	centerLon := baseLon + lonRange/2
	radius := math.Min(latRange, lonRange) / 2 * 0.8

	for i := 0; i < numNodes; i++ {
		angle := 2 * math.Pi * float64(i) / float64(numNodes)
		lat := centerLat + radius*math.Sin(angle)
		lon := centerLon + radius*math.Cos(angle)
		// 添加小量随机噪声
		lat += (rand.Float64() - 0.5) * radius * 0.1
		lon += (rand.Float64() - 0.5) * radius * 0.1
		metrics[i] = g.createNode(i, lat, lon)
	}
	return metrics
}

// 聚类分布 (多个小群)
func (g *DataGenerator) generateCluster(numNodes int, baseLat, baseLon, latRange, lonRange float64) []*models.UAVMetrics {
	metrics := make([]*models.UAVMetrics, numNodes)

	// 2-5个聚类中心
	numClusters := 2 + rand.Intn(4)
	clusterCenters := make([][2]float64, numClusters)
	for c := 0; c < numClusters; c++ {
		clusterCenters[c] = [2]float64{
			baseLat + rand.Float64()*latRange,
			baseLon + rand.Float64()*lonRange,
		}
	}

	clusterRadius := math.Min(latRange, lonRange) / float64(numClusters) * 0.3

	for i := 0; i < numNodes; i++ {
		// 随机选择一个聚类
		cluster := rand.Intn(numClusters)
		lat := clusterCenters[cluster][0] + (rand.Float64()-0.5)*2*clusterRadius
		lon := clusterCenters[cluster][1] + (rand.Float64()-0.5)*2*clusterRadius
		metrics[i] = g.createNode(i, lat, lon)
	}
	return metrics
}

// 创建节点 (随机属性)
func (g *DataGenerator) createNode(index int, lat, lon float64) *models.UAVMetrics {
	return &models.UAVMetrics{
		NodeName: fmt.Sprintf("uav-node-%d", index+1),
		GPS: models.GPSData{
			Latitude:   lat,
			Longitude:  lon,
			Altitude:   50 + rand.Float64()*200,
			LastUpdate: time.Now(),
		},
		Battery: models.BatteryData{
			RemainingPercent: 10 + rand.Float64()*90,
			Voltage:          10.0 + rand.Float64()*2.5,
			Temperature:      20 + rand.Float64()*20,
		},
		Network: &models.NetworkData{
			Latency:        10 + rand.Float64()*200,
			Bandwidth:      20 + rand.Float64()*80,
			PacketLoss:     rand.Float64() * 10,
			SignalStrength: int(-40 - rand.Float64()*50),
		},
		Performance: &models.PerformanceData{
			CPUUsage:    5 + rand.Float64()*80,
			MemoryUsage: 10 + rand.Float64()*70,
			DiskUsage:   5 + rand.Float64()*60,
		},
	}
}

// PrintDatasetStats 打印数据集统计信息
func PrintDatasetStats(data [][]*models.UAVMetrics) {
	fmt.Println("训练数据集统计:")
	fmt.Printf("  总场景数: %d\n", len(data))

	minNodes, maxNodes := len(data[0]), len(data[0])
	for _, d := range data {
		if len(d) < minNodes {
			minNodes = len(d)
		}
		if len(d) > maxNodes {
			maxNodes = len(d)
		}
	}
	fmt.Printf("  节点数范围: %d - %d\n", minNodes, maxNodes)
}

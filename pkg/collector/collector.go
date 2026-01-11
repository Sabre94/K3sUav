package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/k3suav/uav-monitor/pkg/config"
	"github.com/k3suav/uav-monitor/pkg/models"
)

// SimulationData 仿真数据文件格式
type SimulationData struct {
	VMID         string    `json:"vm_id"`
	SimulationID string    `json:"simulation_id"`
	Timestamp    time.Time `json:"timestamp"`
	TimeStep     int       `json:"time_step"`
	Position     struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	} `json:"position"`
	Velocity struct {
		Vx float64 `json:"vx"`
		Vy float64 `json:"vy"`
		Vz float64 `json:"vz"`
	} `json:"velocity"`
	Geodetic struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Altitude  float64 `json:"altitude"`
	} `json:"geodetic"`
	BatteryLevel float64 `json:"battery_level"`
	Heading      float64 `json:"heading"`
	AltitudeAGL  float64 `json:"altitude_agl"`
}

// Collector 仿真数据采集器
type Collector struct {
	config         *config.Config
	simulationPath string
}

// NewCollector 创建采集器
func NewCollector(cfg *config.Config) *Collector {
	return &Collector{
		config:         cfg,
		simulationPath: cfg.Simulation.DataPath,
	}
}

// CollectMetrics 从仿真文件读取数据
func (c *Collector) CollectMetrics(ctx context.Context) (*models.UAVMetrics, error) {
	// 读取仿真数据
	simData, err := c.readSimulationFile()
	if err != nil {
		return nil, fmt.Errorf("failed to read simulation file: %w", err)
	}

	// 计算速度
	speed := math.Sqrt(simData.Velocity.Vx*simData.Velocity.Vx +
		simData.Velocity.Vy*simData.Velocity.Vy +
		simData.Velocity.Vz*simData.Velocity.Vz)

	// 转换电池电量 (0-1 -> 0-100%)
	batteryPercent := simData.BatteryLevel * 100

	// 构建UAVMetrics
	metrics := &models.UAVMetrics{
		NodeName: c.config.Agent.NodeName,
		GPS: models.GPSData{
			Latitude:   simData.Geodetic.Latitude,
			Longitude:  simData.Geodetic.Longitude,
			Altitude:   simData.Geodetic.Altitude,
			Heading:    simData.Heading,
			Speed:      speed,
			Satellites: 12,
			Accuracy:   1.5,
			LastUpdate: time.Now(),
		},
		Battery: models.BatteryData{
			RemainingPercent: batteryPercent,
			Voltage:          11.1 + (batteryPercent/100)*1.5,
			Current:          -5.0 - speed*0.5,
			Temperature:      25 + speed*0.5,
		},
		Position: &models.PositionData{
			X: simData.Position.X,
			Y: simData.Position.Y,
			Z: simData.Position.Z,
		},
		Velocity: &models.VelocityData{
			Vx: simData.Velocity.Vx,
			Vy: simData.Velocity.Vy,
			Vz: simData.Velocity.Vz,
		},
		Simulation: &models.SimulationInfo{
			VMID:         simData.VMID,
			SimulationID: simData.SimulationID,
			TimeStep:     simData.TimeStep,
		},
		Flight: &models.FlightData{
			Armed:         simData.AltitudeAGL > 1.0,
			Mode:          models.FlightModeGuided,
			IsFlying:      simData.AltitudeAGL > 1.0,
			Altitude:      simData.AltitudeAGL,
			VerticalSpeed: simData.Velocity.Vz,
			YawAngle:      simData.Heading,
		},
		Network: &models.NetworkData{
			Latency:        20 + speed*2,
			Bandwidth:      95 - speed*3,
			SignalStrength: -45 - int(speed),
			PacketLoss:     0.1 + speed*0.05,
			ConnectionType: models.ConnectionType5G,
		},
		Performance: &models.PerformanceData{
			CPUUsage:    20 + speed*1.5,
			MemoryUsage: 35 + speed*0.8,
			DiskUsage:   25,
			Temperature: 45 + speed*0.5,
		},
		Health: &models.HealthData{
			Status:          c.getHealthStatus(batteryPercent),
			Errors:          []string{},
			Warnings:        c.getWarnings(batteryPercent),
			LastHealthCheck: time.Now(),
		},
		Metadata: &models.MetadataInfo{
			AgentVersion:    c.config.Agent.Version,
			HardwareModel:   "Simulation-Drone",
			FirmwareVersion: "1.0.0-sim",
			SerialNumber:    simData.VMID,
		},
	}

	return metrics, nil
}

// readSimulationFile 读取仿真JSON文件
func (c *Collector) readSimulationFile() (*SimulationData, error) {
	data, err := os.ReadFile(c.simulationPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var simData SimulationData
	if err := json.Unmarshal(data, &simData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &simData, nil
}

// getHealthStatus 简化的健康状态检查
func (c *Collector) getHealthStatus(batteryPercent float64) string {
	if batteryPercent < 20 {
		return models.HealthStatusCritical
	}
	if batteryPercent < 30 {
		return models.HealthStatusWarning
	}
	return models.HealthStatusHealthy
}

// getWarnings 获取警告信息
func (c *Collector) getWarnings(batteryPercent float64) []string {
	warnings := []string{}
	if batteryPercent < 30 {
		warnings = append(warnings, fmt.Sprintf("Low battery: %.1f%%", batteryPercent))
	}
	return warnings
}

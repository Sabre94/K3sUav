package collector

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/k3suav/uav-monitor/pkg/config"
	"github.com/k3suav/uav-monitor/pkg/models"
)

// Collector collects UAV telemetry data
type Collector struct {
	config     *config.Config
	rand       *rand.Rand
	hostPrefix string // 主机路径前缀（容器中为 /host，宿主机为空）
}

// NewCollector creates a new data collector
func NewCollector(cfg *config.Config) *Collector {
	// 检测是否在容器中运行（通过检查 /host/proc 是否存在）
	hostPrefix := ""
	if _, err := os.Stat("/host/proc"); err == nil {
		hostPrefix = "/host"
	}

	return &Collector{
		config:     cfg,
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
		hostPrefix: hostPrefix,
	}
}

// CollectMetrics collects all enabled metrics
func (c *Collector) CollectMetrics(ctx context.Context) (*models.UAVMetrics, error) {
	metrics := &models.UAVMetrics{
		NodeName: c.config.Agent.NodeName,
	}

	// Collect GPS data
	if c.config.Collection.EnableGPS {
		gps, err := c.collectGPS(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to collect GPS data: %w", err)
		}
		metrics.GPS = *gps
	}

	// Collect battery data
	if c.config.Collection.EnableBattery {
		battery, err := c.collectBattery(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to collect battery data: %w", err)
		}
		metrics.Battery = *battery
	}

	// Collect flight data
	if c.config.Collection.EnableFlight {
		flight, err := c.collectFlight(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to collect flight data: %w", err)
		}
		metrics.Flight = flight
	}

	// Collect network data
	if c.config.Collection.EnableNetwork {
		network, err := c.collectNetwork(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to collect network data: %w", err)
		}
		metrics.Network = network
	}

	// Collect performance data
	if c.config.Collection.EnablePerformance {
		performance, err := c.collectPerformance(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to collect performance data: %w", err)
		}
		metrics.Performance = performance
	}

	// Perform health check
	if c.config.Collection.EnableHealthCheck {
		health := c.performHealthCheck(metrics)
		metrics.Health = health
	}

	// Add metadata
	metrics.Metadata = &models.MetadataInfo{
		AgentVersion:    c.config.Agent.Version,
		HardwareModel:   c.config.UAVMetadata.HardwareModel,
		FirmwareVersion: c.config.UAVMetadata.FirmwareVersion,
		SerialNumber:    c.config.UAVMetadata.SerialNumber,
	}

	return metrics, nil
}

// collectGPS collects GPS data (simulated for now)
func (c *Collector) collectGPS(ctx context.Context) (*models.GPSData, error) {
	// TODO: Integrate with real GPS hardware
	// For now, generate realistic simulated data based on node

	// Use node name to generate consistent location
	seed := int64(0)
	for _, ch := range c.config.Agent.NodeName {
		seed += int64(ch)
	}
	localRand := rand.New(rand.NewSource(seed))

	// Base coordinates (somewhere in California)
	baseLat := 34.0522 + localRand.Float64()*0.1
	baseLon := -118.2437 + localRand.Float64()*0.1

	// Add some variation for movement
	gps := &models.GPSData{
		Latitude:   baseLat + (c.rand.Float64()-0.5)*0.001,
		Longitude:  baseLon + (c.rand.Float64()-0.5)*0.001,
		Altitude:   50 + c.rand.Float64()*100,
		Heading:    c.rand.Float64() * 360,
		Speed:      c.rand.Float64() * 15, // 0-15 m/s
		Satellites: 8 + c.rand.Intn(5),    // 8-12 satellites
		Accuracy:   2 + c.rand.Float64()*3, // 2-5 meters
		LastUpdate: time.Now(),
	}

	// Validate GPS data
	if err := gps.ValidateGPS(); err != nil {
		return nil, err
	}

	return gps, nil
}

// collectBattery collects battery data
func (c *Collector) collectBattery(ctx context.Context) (*models.BatteryData, error) {
	// Try to read from system power supply
	remainingPercent, err := c.readBatteryFromSystem()
	if err != nil {
		// Fall back to simulated data
		remainingPercent = 50 + c.rand.Float64()*50 // 50-100%
	}

	battery := &models.BatteryData{
		RemainingPercent: remainingPercent,
		Voltage:          11.1 + (remainingPercent/100)*1.5, // 11.1V-12.6V for 3S LiPo
		Current:          -5.0 - c.rand.Float64()*5.0,        // -5 to -10A when flying
		Temperature:      20 + c.rand.Float64()*15,           // 20-35°C
		TimeRemaining:    int((remainingPercent / 100) * 1800), // Estimate 30 min max flight time
		CycleCount:       50 + c.rand.Intn(200),
	}

	// Validate battery data
	if err := battery.ValidateBattery(); err != nil {
		return nil, err
	}

	return battery, nil
}

// collectFlight collects flight data
func (c *Collector) collectFlight(ctx context.Context) (*models.FlightData, error) {
	modes := []string{
		models.FlightModeStabilize,
		models.FlightModeAltitudeHold,
		models.FlightModePositionHold,
		models.FlightModeGuided,
	}

	flight := &models.FlightData{
		Armed:         c.rand.Float64() > 0.3, // 70% chance armed
		Mode:          modes[c.rand.Intn(len(modes))],
		IsFlying:      c.rand.Float64() > 0.4, // 60% chance flying
		Altitude:      c.rand.Float64() * 100,  // 0-100m
		VerticalSpeed: (c.rand.Float64() - 0.5) * 4, // -2 to 2 m/s
		RollAngle:     (c.rand.Float64() - 0.5) * 30, // -15 to 15 degrees
		PitchAngle:    (c.rand.Float64() - 0.5) * 30, // -15 to 15 degrees
		YawAngle:      c.rand.Float64() * 360,         // 0-360 degrees
	}

	return flight, nil
}

// collectNetwork collects network data
func (c *Collector) collectNetwork(ctx context.Context) (*models.NetworkData, error) {
	// Try to measure real latency
	latency := c.measureLatency()

	connectionTypes := []string{
		models.ConnectionType4G,
		models.ConnectionType5G,
		models.ConnectionTypeWiFi,
	}

	network := &models.NetworkData{
		Latency:        latency,
		Bandwidth:      10 + c.rand.Float64()*90, // 10-100 Mbps
		SignalStrength: -40 - c.rand.Intn(40),     // -40 to -80 dBm
		PacketLoss:     c.rand.Float64() * 2,      // 0-2%
		ConnectionType: connectionTypes[c.rand.Intn(len(connectionTypes))],
	}

	return network, nil
}

// collectPerformance collects system performance data
func (c *Collector) collectPerformance(ctx context.Context) (*models.PerformanceData, error) {
	// Try to read real CPU usage
	cpuUsage, _ := c.readCPUUsage()
	if cpuUsage == 0 {
		cpuUsage = 10 + c.rand.Float64()*40 // 10-50% simulated
	}

	// Try to read real memory usage
	memUsage, _ := c.readMemoryUsage()
	if memUsage == 0 {
		memUsage = 30 + c.rand.Float64()*30 // 30-60% simulated
	}

	// Read system uptime
	uptime, _ := c.readSystemUptime()

	performance := &models.PerformanceData{
		CPUUsage:    cpuUsage,
		MemoryUsage: memUsage,
		DiskUsage:   20 + c.rand.Float64()*30, // 20-50%
		Temperature: 40 + c.rand.Float64()*20,  // 40-60°C
		Uptime:      uptime,
	}

	return performance, nil
}

// performHealthCheck evaluates overall health
func (c *Collector) performHealthCheck(metrics *models.UAVMetrics) *models.HealthData {
	health := &models.HealthData{
		Status:          models.HealthStatusHealthy,
		Errors:          []string{},
		Warnings:        []string{},
		LastHealthCheck: time.Now(),
	}

	// Check battery
	if metrics.Battery.IsCriticalBattery() {
		health.Status = models.HealthStatusCritical
		health.Errors = append(health.Errors, fmt.Sprintf("Critical battery: %.1f%%", metrics.Battery.RemainingPercent))
	} else if metrics.Battery.IsLowBattery(c.config.Collection.BatteryLowThreshold) {
		if health.Status != models.HealthStatusCritical {
			health.Status = models.HealthStatusWarning
		}
		health.Warnings = append(health.Warnings, fmt.Sprintf("Low battery: %.1f%%", metrics.Battery.RemainingPercent))
	}

	// Check GPS
	if metrics.GPS.Satellites < c.config.Collection.GPSMinSatellites {
		health.Warnings = append(health.Warnings, fmt.Sprintf("Low GPS satellites: %d", metrics.GPS.Satellites))
		if health.Status == models.HealthStatusHealthy {
			health.Status = models.HealthStatusWarning
		}
	}

	// Check network
	if metrics.Network != nil && metrics.Network.Latency > 200 {
		health.Warnings = append(health.Warnings, fmt.Sprintf("High latency: %.1fms", metrics.Network.Latency))
		if health.Status == models.HealthStatusHealthy {
			health.Status = models.HealthStatusWarning
		}
	}

	// Check performance
	if metrics.Performance != nil && metrics.Performance.CPUUsage > 80 {
		health.Warnings = append(health.Warnings, fmt.Sprintf("High CPU usage: %.1f%%", metrics.Performance.CPUUsage))
		if health.Status == models.HealthStatusHealthy {
			health.Status = models.HealthStatusWarning
		}
	}

	return health
}

// System reading helper functions

func (c *Collector) readBatteryFromSystem() (float64, error) {
	// Try to read from /sys/class/power_supply/BAT0/capacity
	batteryPath := c.hostPrefix + "/sys/class/power_supply/BAT0/capacity"
	data, err := os.ReadFile(batteryPath)
	if err != nil {
		return 0, err
	}

	capacity, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return 0, err
	}

	return capacity, nil
}

func (c *Collector) readCPUUsage() (float64, error) {
	// Simple CPU usage calculation from /proc/stat
	// This is simplified - a real implementation would calculate delta
	statPath := c.hostPrefix + "/proc/stat"
	file, err := os.Open(statPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) > 4 && fields[0] == "cpu" {
			// Calculate a rough percentage
			// For real usage, you'd need to calculate delta between two reads
			return c.rand.Float64() * 50, nil // Placeholder
		}
	}

	return 0, fmt.Errorf("failed to read CPU stats")
}

func (c *Collector) readMemoryUsage() (float64, error) {
	// Read from /proc/meminfo
	meminfoPath := c.hostPrefix + "/proc/meminfo"
	file, err := os.Open(meminfoPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var memTotal, memAvailable float64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		if fields[0] == "MemTotal:" {
			memTotal, _ = strconv.ParseFloat(fields[1], 64)
		} else if fields[0] == "MemAvailable:" {
			memAvailable, _ = strconv.ParseFloat(fields[1], 64)
		}
	}

	if memTotal > 0 {
		usage := ((memTotal - memAvailable) / memTotal) * 100
		return usage, nil
	}

	return 0, fmt.Errorf("failed to read memory stats")
}

func (c *Collector) readSystemUptime() (int64, error) {
	// Read from /proc/uptime
	uptimePath := c.hostPrefix + "/proc/uptime"
	data, err := os.ReadFile(uptimePath)
	if err != nil {
		return 0, err
	}

	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("invalid uptime format")
	}

	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}

	return int64(uptime), nil
}

func (c *Collector) measureLatency() float64 {
	// Simple ping simulation - in production, you'd actually ping a server
	// For now, return a random value with some variation
	baseLatency := 50.0 // 50ms base
	variation := c.rand.Float64() * 100 // 0-100ms variation
	return baseLatency + variation
}

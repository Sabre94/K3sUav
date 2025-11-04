package models

import (
	"time"
)

// UAVMetrics represents the complete metrics data for a UAV
type UAVMetrics struct {
	NodeName    string            `json:"nodeName"`
	GPS         GPSData           `json:"gps"`
	Battery     BatteryData       `json:"battery"`
	Flight      *FlightData       `json:"flight,omitempty"`
	Network     *NetworkData      `json:"network,omitempty"`
	Performance *PerformanceData  `json:"performance,omitempty"`
	Health      *HealthData       `json:"health,omitempty"`
	Metadata    *MetadataInfo     `json:"metadata,omitempty"`
}

// GPSData contains GPS location information
type GPSData struct {
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	Altitude   float64   `json:"altitude,omitempty"`
	Heading    float64   `json:"heading,omitempty"`
	Speed      float64   `json:"speed,omitempty"`
	Satellites int       `json:"satellites,omitempty"`
	Accuracy   float64   `json:"accuracy,omitempty"`
	LastUpdate time.Time `json:"lastUpdate"`
}

// BatteryData contains battery information
type BatteryData struct {
	RemainingPercent float64 `json:"remainingPercent"`
	Voltage          float64 `json:"voltage,omitempty"`
	Current          float64 `json:"current,omitempty"`
	Temperature      float64 `json:"temperature,omitempty"`
	TimeRemaining    int     `json:"timeRemaining,omitempty"`
	CycleCount       int     `json:"cycleCount,omitempty"`
}

// FlightData contains flight status information
type FlightData struct {
	Armed         bool    `json:"armed"`
	Mode          string  `json:"mode"`
	IsFlying      bool    `json:"isFlying"`
	Altitude      float64 `json:"altitude,omitempty"`
	VerticalSpeed float64 `json:"verticalSpeed,omitempty"`
	RollAngle     float64 `json:"rollAngle,omitempty"`
	PitchAngle    float64 `json:"pitchAngle,omitempty"`
	YawAngle      float64 `json:"yawAngle,omitempty"`
}

// NetworkData contains network information
type NetworkData struct {
	Latency        float64 `json:"latency,omitempty"`
	Bandwidth      float64 `json:"bandwidth,omitempty"`
	SignalStrength int     `json:"signalStrength,omitempty"`
	PacketLoss     float64 `json:"packetLoss,omitempty"`
	ConnectionType string  `json:"connectionType,omitempty"`
}

// PerformanceData contains system performance metrics
type PerformanceData struct {
	CPUUsage    float64 `json:"cpuUsage,omitempty"`
	MemoryUsage float64 `json:"memoryUsage,omitempty"`
	DiskUsage   float64 `json:"diskUsage,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	Uptime      int64   `json:"uptime,omitempty"`
}

// HealthData contains health status information
type HealthData struct {
	Status          string    `json:"status"`
	Errors          []string  `json:"errors,omitempty"`
	Warnings        []string  `json:"warnings,omitempty"`
	LastHealthCheck time.Time `json:"lastHealthCheck"`
}

// MetadataInfo contains UAV metadata
type MetadataInfo struct {
	AgentVersion    string `json:"agentVersion,omitempty"`
	HardwareModel   string `json:"hardwareModel,omitempty"`
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	SerialNumber    string `json:"serialNumber,omitempty"`
}

// HealthStatus constants
const (
	HealthStatusHealthy  = "Healthy"
	HealthStatusWarning  = "Warning"
	HealthStatusCritical = "Critical"
	HealthStatusUnknown  = "Unknown"
)

// FlightMode constants
const (
	FlightModeManual       = "MANUAL"
	FlightModeStabilize    = "STABILIZE"
	FlightModeAltitudeHold = "ALTITUDE_HOLD"
	FlightModePositionHold = "POSITION_HOLD"
	FlightModeAuto         = "AUTO"
	FlightModeGuided       = "GUIDED"
	FlightModeLoiter       = "LOITER"
	FlightModeRTL          = "RTL"
	FlightModeLand         = "LAND"
	FlightModeUnknown      = "UNKNOWN"
)

// ConnectionType constants
const (
	ConnectionType4G        = "4G"
	ConnectionType5G        = "5G"
	ConnectionTypeWiFi      = "WIFI"
	ConnectionTypeSatellite = "SATELLITE"
	ConnectionTypeUnknown   = "UNKNOWN"
)

// ValidateGPS validates GPS data
func (g *GPSData) ValidateGPS() error {
	if g.Latitude < -90 || g.Latitude > 90 {
		return ErrInvalidLatitude
	}
	if g.Longitude < -180 || g.Longitude > 180 {
		return ErrInvalidLongitude
	}
	return nil
}

// ValidateBattery validates battery data
func (b *BatteryData) ValidateBattery() error {
	if b.RemainingPercent < 0 || b.RemainingPercent > 100 {
		return ErrInvalidBatteryPercent
	}
	return nil
}

// IsLowBattery checks if battery is below threshold
func (b *BatteryData) IsLowBattery(threshold float64) bool {
	return b.RemainingPercent < threshold
}

// IsCriticalBattery checks if battery is critically low
func (b *BatteryData) IsCriticalBattery() bool {
	return b.RemainingPercent < 20.0
}

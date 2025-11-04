package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds the configuration for the UAV agent
type Config struct {
	// Agent configuration
	Agent AgentConfig `json:"agent"`

	// Kubernetes configuration
	Kubernetes K8sConfig `json:"kubernetes"`

	// Data collection configuration
	Collection CollectionConfig `json:"collection"`

	// UAV metadata
	UAVMetadata UAVMetadataConfig `json:"uavMetadata"`
}

// AgentConfig contains agent-specific settings
type AgentConfig struct {
	// Node name (auto-detected from K8s)
	NodeName string `json:"nodeName"`

	// Agent version
	Version string `json:"version"`

	// Log level (debug, info, warn, error)
	LogLevel string `json:"logLevel"`

	// Enable structured logging
	StructuredLogging bool `json:"structuredLogging"`
}

// K8sConfig contains Kubernetes client settings
type K8sConfig struct {
	// Kubeconfig path (empty for in-cluster config)
	KubeconfigPath string `json:"kubeconfigPath"`

	// Namespace for UAV resources
	Namespace string `json:"namespace"`

	// CRD name
	CRDName string `json:"crdName"`

	// CRD Group
	CRDGroup string `json:"crdGroup"`

	// CRD Version
	CRDVersion string `json:"crdVersion"`

	// Update retry attempts
	RetryAttempts int `json:"retryAttempts"`

	// Retry delay
	RetryDelay time.Duration `json:"retryDelay"`
}

// CollectionConfig contains data collection settings
type CollectionConfig struct {
	// Collection interval
	Interval time.Duration `json:"interval"`

	// GPS collection enabled
	EnableGPS bool `json:"enableGPS"`

	// Battery collection enabled
	EnableBattery bool `json:"enableBattery"`

	// Flight data collection enabled
	EnableFlight bool `json:"enableFlight"`

	// Network data collection enabled
	EnableNetwork bool `json:"enableNetwork"`

	// Performance data collection enabled
	EnablePerformance bool `json:"enablePerformance"`

	// Health check enabled
	EnableHealthCheck bool `json:"enableHealthCheck"`

	// Battery low threshold
	BatteryLowThreshold float64 `json:"batteryLowThreshold"`

	// Battery critical threshold
	BatteryCriticalThreshold float64 `json:"batteryCriticalThreshold"`

	// GPS minimum satellites
	GPSMinSatellites int `json:"gpsMinSatellites"`
}

// UAVMetadataConfig contains UAV hardware metadata
type UAVMetadataConfig struct {
	// Hardware model
	HardwareModel string `json:"hardwareModel"`

	// Firmware version
	FirmwareVersion string `json:"firmwareVersion"`

	// Serial number
	SerialNumber string `json:"serialNumber"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Agent: AgentConfig{
			NodeName:          getEnvOrDefault("NODE_NAME", ""),
			Version:           "v0.1.0",
			LogLevel:          getEnvOrDefault("LOG_LEVEL", "info"),
			StructuredLogging: true,
		},
		Kubernetes: K8sConfig{
			KubeconfigPath: getEnvOrDefault("KUBECONFIG", ""),
			Namespace:      getEnvOrDefault("NAMESPACE", "default"),
			CRDName:        "uavmetrics.uav.k3s.io",
			CRDGroup:       "uav.k3s.io",
			CRDVersion:     "v1alpha1",
			RetryAttempts:  3,
			RetryDelay:     2 * time.Second,
		},
		Collection: CollectionConfig{
			Interval:                 getEnvDurationOrDefault("COLLECTION_INTERVAL", 10*time.Second),
			EnableGPS:                getEnvBoolOrDefault("ENABLE_GPS", true),
			EnableBattery:            getEnvBoolOrDefault("ENABLE_BATTERY", true),
			EnableFlight:             getEnvBoolOrDefault("ENABLE_FLIGHT", true),
			EnableNetwork:            getEnvBoolOrDefault("ENABLE_NETWORK", true),
			EnablePerformance:        getEnvBoolOrDefault("ENABLE_PERFORMANCE", true),
			EnableHealthCheck:        getEnvBoolOrDefault("ENABLE_HEALTH_CHECK", true),
			BatteryLowThreshold:      30.0,
			BatteryCriticalThreshold: 20.0,
			GPSMinSatellites:         4,
		},
		UAVMetadata: UAVMetadataConfig{
			HardwareModel:   getEnvOrDefault("UAV_HARDWARE_MODEL", "Generic-UAV-v1"),
			FirmwareVersion: getEnvOrDefault("UAV_FIRMWARE_VERSION", "1.0.0"),
			SerialNumber:    getEnvOrDefault("UAV_SERIAL_NUMBER", "UAV-000000"),
		},
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate agent config
	if c.Agent.NodeName == "" {
		return fmt.Errorf("agent.nodeName is required (set NODE_NAME environment variable)")
	}

	// Validate Kubernetes config
	if c.Kubernetes.Namespace == "" {
		return fmt.Errorf("kubernetes.namespace cannot be empty")
	}
	if c.Kubernetes.CRDName == "" {
		return fmt.Errorf("kubernetes.crdName cannot be empty")
	}
	if c.Kubernetes.RetryAttempts < 0 {
		return fmt.Errorf("kubernetes.retryAttempts must be >= 0")
	}

	// Validate collection config
	if c.Collection.Interval <= 0 {
		return fmt.Errorf("collection.interval must be > 0")
	}
	if c.Collection.BatteryLowThreshold < 0 || c.Collection.BatteryLowThreshold > 100 {
		return fmt.Errorf("collection.batteryLowThreshold must be between 0 and 100")
	}
	if c.Collection.BatteryCriticalThreshold < 0 || c.Collection.BatteryCriticalThreshold > 100 {
		return fmt.Errorf("collection.batteryCriticalThreshold must be between 0 and 100")
	}
	if c.Collection.GPSMinSatellites < 0 {
		return fmt.Errorf("collection.gpsMinSatellites must be >= 0")
	}

	return nil
}

// Helper functions

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value == "true" || value == "1" || value == "yes"
}

func getEnvDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	return duration
}

package collector

import (
	"math"
	"sync"
	"time"

	"github.com/k3suav/uav-monitor/pkg/config"
	"github.com/k3suav/uav-monitor/pkg/models"
)

// ChangeDetector detects significant changes in UAV metrics
type ChangeDetector struct {
	config *config.Config

	// Mutex for thread-safe access
	mu sync.RWMutex

	// Last pushed metrics
	lastMetrics    *models.UAVMetrics
	lastUpdateTime time.Time

	// Statistics
	totalSamples      int64
	pushedSamples     int64
	positionChanges   int64
	batteryChanges    int64
	timeoutPushes     int64
}

// NewChangeDetector creates a new change detector
func NewChangeDetector(cfg *config.Config) *ChangeDetector {
	return &ChangeDetector{
		config:         cfg,
		lastUpdateTime: time.Time{}, // Zero time forces first push
	}
}

// ShouldUpdate determines if metrics should be pushed to K8s
func (cd *ChangeDetector) ShouldUpdate(metrics *models.UAVMetrics) (bool, string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	cd.totalSamples++

	// First update always goes through
	if cd.lastMetrics == nil {
		cd.recordUpdate(metrics, "initial")
		return true, "initial update"
	}

	now := time.Now()
	timeSinceLastUpdate := now.Sub(cd.lastUpdateTime)

	// Check minimum interval - prevent too frequent updates
	if timeSinceLastUpdate < cd.config.Collection.MinUpdateInterval {
		return false, "min interval not reached"
	}

	// Check maximum interval - force update
	if timeSinceLastUpdate >= cd.config.Collection.MaxUpdateInterval {
		cd.recordUpdate(metrics, "timeout")
		cd.timeoutPushes++
		return true, "max interval reached (forced update)"
	}

	// Change detection disabled - use time-based only
	if !cd.config.Collection.EnableChangeDetection {
		if timeSinceLastUpdate >= cd.config.Collection.Interval {
			cd.recordUpdate(metrics, "interval")
			return true, "collection interval reached"
		}
		return false, "interval not reached"
	}

	// Check position change (GPS or Cartesian)
	if cd.hasSignificantPositionChange(metrics) {
		cd.recordUpdate(metrics, "position")
		cd.positionChanges++
		return true, "position change exceeds threshold"
	}

	// Check battery change
	if cd.hasSignificantBatteryChange(metrics) {
		cd.recordUpdate(metrics, "battery")
		cd.batteryChanges++
		return true, "battery change exceeds threshold"
	}

	return false, "no significant change"
}

// hasSignificantPositionChange checks if position changed beyond threshold
func (cd *ChangeDetector) hasSignificantPositionChange(metrics *models.UAVMetrics) bool {
	// Priority 1: Use Cartesian position if available (more accurate for simulation)
	if metrics.Position != nil && cd.lastMetrics.Position != nil {
		distance := calculateCartesianDistance(
			cd.lastMetrics.Position.X, cd.lastMetrics.Position.Y, cd.lastMetrics.Position.Z,
			metrics.Position.X, metrics.Position.Y, metrics.Position.Z,
		)
		return distance >= cd.config.Collection.PositionChangeThreshold
	}

	// Priority 2: Use GPS coordinates
	if cd.config.Collection.EnableGPS {
		distance := calculateGPSDistance(
			cd.lastMetrics.GPS.Latitude, cd.lastMetrics.GPS.Longitude,
			metrics.GPS.Latitude, metrics.GPS.Longitude,
		)
		return distance >= cd.config.Collection.PositionChangeThreshold
	}

	return false
}

// hasSignificantBatteryChange checks if battery changed beyond threshold
func (cd *ChangeDetector) hasSignificantBatteryChange(metrics *models.UAVMetrics) bool {
	if !cd.config.Collection.EnableBattery {
		return false
	}

	batteryDelta := math.Abs(cd.lastMetrics.Battery.RemainingPercent - metrics.Battery.RemainingPercent)
	return batteryDelta >= cd.config.Collection.BatteryChangeThreshold
}

// recordUpdate records that an update was pushed
func (cd *ChangeDetector) recordUpdate(metrics *models.UAVMetrics, reason string) {
	cd.lastMetrics = metrics
	cd.lastUpdateTime = time.Now()
	cd.pushedSamples++
}

// GetStatistics returns change detection statistics
func (cd *ChangeDetector) GetStatistics() map[string]interface{} {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	pushRate := 0.0
	if cd.totalSamples > 0 {
		pushRate = float64(cd.pushedSamples) / float64(cd.totalSamples) * 100
	}

	return map[string]interface{}{
		"total_samples":    cd.totalSamples,
		"pushed_samples":   cd.pushedSamples,
		"push_rate_pct":    pushRate,
		"position_changes": cd.positionChanges,
		"battery_changes":  cd.batteryChanges,
		"timeout_pushes":   cd.timeoutPushes,
	}
}

// calculateCartesianDistance calculates 3D Euclidean distance in meters
func calculateCartesianDistance(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	dz := z2 - z1
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// calculateGPSDistance calculates distance between two GPS coordinates using Haversine formula
// Returns distance in meters
func calculateGPSDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0 // meters

	// Convert to radians
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	// Haversine formula
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

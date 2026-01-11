package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/k3suav/uav-monitor/pkg/collector"
	"github.com/k3suav/uav-monitor/pkg/config"
	"github.com/k3suav/uav-monitor/pkg/k8s"
	"github.com/k3suav/uav-monitor/pkg/models"
	"github.com/sirupsen/logrus"
)

const (
	version = "v0.1.0"
)

var (
	log = logrus.New()
)

func main() {
	// Initialize logger
	initLogger()

	log.WithField("version", version).Info("Starting UAV Agent")

	// Load configuration
	cfg := config.DefaultConfig()
	if err := cfg.Validate(); err != nil {
		log.WithError(err).Fatal("Invalid configuration")
	}

	log.WithFields(logrus.Fields{
		"nodeName":           cfg.Agent.NodeName,
		"namespace":          cfg.Kubernetes.Namespace,
		"collectionInterval": cfg.Collection.Interval,
		"simulationMode":     cfg.Simulation.Enabled,
		"changeDetection":    cfg.Collection.EnableChangeDetection,
	}).Info("Configuration loaded")

	// Create Kubernetes client
	k8sClient, err := k8s.NewClient(cfg)
	if err != nil {
		log.WithError(err).Fatal("Failed to create Kubernetes client")
	}
	log.Info("Kubernetes client initialized")

	// Create change detector
	changeDetector := collector.NewChangeDetector(cfg)
	log.WithFields(logrus.Fields{
		"enabled":           cfg.Collection.EnableChangeDetection,
		"positionThreshold": fmt.Sprintf("%.1fm", cfg.Collection.PositionChangeThreshold),
		"batteryThreshold":  fmt.Sprintf("%.1f%%", cfg.Collection.BatteryChangeThreshold),
		"minUpdateInterval": cfg.Collection.MinUpdateInterval,
		"maxUpdateInterval": cfg.Collection.MaxUpdateInterval,
	}).Info("Change detector initialized")

	// Create data collector
	dataCollector := collector.NewCollector(cfg)
	if cfg.Simulation.Enabled {
		log.WithField("dataPath", cfg.Simulation.DataPath).Info("Simulation mode enabled")
	} else {
		log.Info("Real-time mode enabled")
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create error channel for goroutines
	errChan := make(chan error, 1)

	// Start collection loop in goroutine
	go func() {
		errChan <- runCollectionLoop(ctx, cfg, k8sClient, dataCollector, changeDetector)
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		log.WithField("signal", sig).Info("Received shutdown signal")
		cancel()
	case err := <-errChan:
		if err != nil {
			log.WithError(err).Error("Collection loop error")
			cancel()
		}
	}

	// Graceful shutdown
	log.Info("Shutting down gracefully...")

	// Update status to Inactive before shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := k8sClient.UpdateStatus(shutdownCtx, cfg.Agent.NodeName, "Inactive"); err != nil {
		log.WithError(err).Warn("Failed to update status on shutdown")
	}

	log.Info("UAV Agent stopped")
}

func runCollectionLoop(ctx context.Context, cfg *config.Config, k8sClient *k8s.Client, dataCollector interface {
	CollectMetrics(ctx context.Context) (*models.UAVMetrics, error)
}, changeDetector *collector.ChangeDetector) error {
	ticker := time.NewTicker(cfg.Collection.Interval)
	defer ticker.Stop()

	// Initial collection and push
	if err := collectAndUpdate(ctx, cfg, k8sClient, dataCollector, changeDetector, true); err != nil {
		log.WithError(err).Error("Initial collection failed")
	}

	// Statistics logging ticker
	statsTicker := time.NewTicker(60 * time.Second) // Log stats every 60s
	defer statsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Collection loop stopped")
			// Log final statistics
			logChangeDetectionStats(changeDetector)
			return ctx.Err()
		case <-ticker.C:
			if err := collectAndUpdate(ctx, cfg, k8sClient, dataCollector, changeDetector, false); err != nil {
				log.WithError(err).Error("Collection failed")
				// Continue despite errors - don't stop the loop
			}
		case <-statsTicker.C:
			// Periodically log change detection statistics
			logChangeDetectionStats(changeDetector)
		}
	}
}

func collectAndUpdate(ctx context.Context, cfg *config.Config, k8sClient *k8s.Client, dataCollector interface {
	CollectMetrics(ctx context.Context) (*models.UAVMetrics, error)
}, changeDetector *collector.ChangeDetector, forceUpdate bool) error {
	startTime := time.Now()

	// Collect metrics
	metrics, err := dataCollector.CollectMetrics(ctx)
	if err != nil {
		return fmt.Errorf("failed to collect metrics: %w", err)
	}

	collectionDuration := time.Since(startTime)

	// Check if we should update based on changes (unless forced)
	shouldUpdate := forceUpdate
	updateReason := "forced"

	if !forceUpdate {
		var reason string
		shouldUpdate, reason = changeDetector.ShouldUpdate(metrics)
		updateReason = reason

		log.WithFields(logrus.Fields{
			"nodeName":     metrics.NodeName,
			"battery":      fmt.Sprintf("%.1f%%", metrics.Battery.RemainingPercent),
			"shouldUpdate": shouldUpdate,
			"reason":       reason,
		}).Debug("Change detection result")

		if !shouldUpdate {
			// Skip update - no significant change
			return nil
		}
	}

	log.WithFields(logrus.Fields{
		"nodeName":    metrics.NodeName,
		"battery":     fmt.Sprintf("%.1f%%", metrics.Battery.RemainingPercent),
		"gps_lat":     fmt.Sprintf("%.6f", metrics.GPS.Latitude),
		"gps_lon":     fmt.Sprintf("%.6f", metrics.GPS.Longitude),
		"gps_sats":    metrics.GPS.Satellites,
		"health":      metrics.Health.Status,
		"duration_ms": collectionDuration.Milliseconds(),
		"reason":      updateReason,
	}).Debug("Metrics collected - pushing update")

	// Update CRD with retry
	updateStart := time.Now()
	if err := k8sClient.CreateOrUpdateWithRetry(ctx, metrics); err != nil {
		return fmt.Errorf("failed to update CRD: %w", err)
	}
	updateDuration := time.Since(updateStart)

	// Determine phase based on health
	phase := "Active"
	if metrics.Health != nil {
		switch metrics.Health.Status {
		case "Critical":
			phase = "Error"
		case "Warning", "Healthy":
			phase = "Active"
		default:
			phase = "Unknown"
		}
	}

	// Update status
	if err := k8sClient.UpdateStatus(ctx, metrics.NodeName, phase); err != nil {
		log.WithError(err).Warn("Failed to update status")
		// Don't return error for status update failures
	}

	totalDuration := time.Since(startTime)

	log.WithFields(logrus.Fields{
		"nodeName":      metrics.NodeName,
		"battery":       fmt.Sprintf("%.1f%%", metrics.Battery.RemainingPercent),
		"health":        metrics.Health.Status,
		"errors":        len(metrics.Health.Errors),
		"warnings":      len(metrics.Health.Warnings),
		"collection_ms": collectionDuration.Milliseconds(),
		"update_ms":     updateDuration.Milliseconds(),
		"total_ms":      totalDuration.Milliseconds(),
		"reason":        updateReason,
	}).Info("Metrics updated successfully")

	// Log warnings and errors
	if metrics.Health != nil {
		for _, warning := range metrics.Health.Warnings {
			log.WithField("warning", warning).Warn("Health warning")
		}
		for _, errMsg := range metrics.Health.Errors {
			log.WithField("error", errMsg).Error("Health error")
		}
	}

	return nil
}

func logChangeDetectionStats(changeDetector *collector.ChangeDetector) {
	stats := changeDetector.GetStatistics()
	log.WithFields(logrus.Fields{
		"total_samples":    stats["total_samples"],
		"pushed_samples":   stats["pushed_samples"],
		"push_rate":        fmt.Sprintf("%.1f%%", stats["push_rate_pct"]),
		"position_changes": stats["position_changes"],
		"battery_changes":  stats["battery_changes"],
		"timeout_pushes":   stats["timeout_pushes"],
	}).Info("Change detection statistics")
}

func initLogger() {
	// Set log format
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	// Set log level from environment or default to Info
	logLevel := os.Getenv("LOG_LEVEL")
	switch logLevel {
	case "debug":
		log.SetLevel(logrus.DebugLevel)
	case "info":
		log.SetLevel(logrus.InfoLevel)
	case "warn":
		log.SetLevel(logrus.WarnLevel)
	case "error":
		log.SetLevel(logrus.ErrorLevel)
	default:
		log.SetLevel(logrus.InfoLevel)
	}

	// Use JSON format if structured logging is enabled
	if os.Getenv("STRUCTURED_LOGGING") == "true" {
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	}

	log.SetOutput(os.Stdout)
}

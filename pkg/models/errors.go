package models

import "errors"

var (
	// GPS errors
	ErrInvalidLatitude  = errors.New("invalid latitude: must be between -90 and 90")
	ErrInvalidLongitude = errors.New("invalid longitude: must be between -180 and 180")
	ErrGPSNotLocked     = errors.New("GPS not locked: insufficient satellites")

	// Battery errors
	ErrInvalidBatteryPercent = errors.New("invalid battery percentage: must be between 0 and 100")
	ErrCriticalBattery       = errors.New("critical battery level: below 20%")
	ErrBatteryNotAvailable   = errors.New("battery data not available")

	// Collection errors
	ErrCollectionFailed = errors.New("data collection failed")
	ErrNoDataAvailable  = errors.New("no data available to collect")

	// K8s errors
	ErrK8sClientNotInitialized = errors.New("kubernetes client not initialized")
	ErrCRDUpdateFailed         = errors.New("failed to update CRD")
	ErrCRDNotFound             = errors.New("CRD not found")
)

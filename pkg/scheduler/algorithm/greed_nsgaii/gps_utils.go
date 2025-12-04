package greed_nsgaii

import (
	"math"
)

const (
	EarthRadiusMeters     = 6371000.0 // 地球半径（米）
	MetersPerLatDegree    = 111000.0  // 纬度每度对应的米数（近似）
)

// GPSConverter GPS 坐标转换器
type GPSConverter struct {
	masterLat float64
	masterLon float64
	metersPerLonDegree float64 // 经度每度对应的米数（根据纬度计算）
}

// NewGPSConverter 创建 GPS 转换器
func NewGPSConverter(masterLat, masterLon float64) *GPSConverter {
	metersPerLonDegree := MetersPerLatDegree * math.Cos(masterLat*math.Pi/180)
	return &GPSConverter{
		masterLat:          masterLat,
		masterLon:          masterLon,
		metersPerLonDegree: metersPerLonDegree,
	}
}

// GPSToXY 将 GPS 坐标转换为相对 XY 坐标（米）
func (g *GPSConverter) GPSToXY(targetLat, targetLon float64) (x, y float64) {
	deltaLat := targetLat - g.masterLat
	deltaLon := targetLon - g.masterLon

	y = deltaLat * MetersPerLatDegree
	x = deltaLon * g.metersPerLonDegree

	return x, y
}

// XYToGPS 将相对 XY 坐标（米）转换为 GPS 坐标
func (g *GPSConverter) XYToGPS(x, y float64) (lat, lon float64) {
	lat = g.masterLat + (y / MetersPerLatDegree)
	lon = g.masterLon + (x / g.metersPerLonDegree)
	return lat, lon
}

// HaversineDistance 计算两个 GPS 坐标之间的距离（米）
// 使用 Haversine 公式
func HaversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// 转换为弧度
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	// Haversine 公式
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return EarthRadiusMeters * c
}

// EuclideanDistance 计算两个 XY 坐标之间的欧几里得距离（米）
func EuclideanDistance(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	return math.Sqrt(dx*dx + dy*dy)
}

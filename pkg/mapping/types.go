package mapping

import "math"

type MetricType int

const (
	Gauge MetricType = iota
	Counter
)

// sentinelFloat is the threshold above which a float64/float32 value is treated
// as an SMA "not available" sentinel artifact rather than a real measurement.
//
// SMA marks unavailable values with all-ones/min-int sentinels; sunny's uint64
// sentinels survive and its kWh-alias math inflates them to ~1.8e16 — no real
// quantity this exporter measures (W, var, VA, V, A, Hz, %, °C, kWh) ever
// reaches 1e15, so anything at/above that is a sentinel artifact.
const sentinelFloat = 1e15

// Snapshot is a single mapped metric value with a bare (prefix-less) name.
type Snapshot struct {
	Name   string
	Type   MetricType
	Labels map[string]string
	Value  float64
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) >= sentinelFloat {
			return 0, false
		}
		return n, true
	case float32:
		f := float64(n)
		if math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) >= sentinelFloat {
			return 0, false
		}
		return f, true
	case uint64:
		if n == math.MaxUint64 {
			return 0, false
		}
		return float64(n), true
	case uint32:
		if n == math.MaxUint32 {
			return 0, false
		}
		return float64(n), true
	case int64:
		if n == math.MinInt64 {
			return 0, false
		}
		return float64(n), true
	case int32:
		if n == math.MinInt32 {
			return 0, false
		}
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

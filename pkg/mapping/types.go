package mapping

type MetricType int

const (
	Gauge MetricType = iota
	Counter
)

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
		return n, true
	case float32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case uint32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

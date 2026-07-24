package collector

import (
	"testing"
	"time"

	"github.com/chr-fritz/speedwire-exporter/pkg/mapping"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestCollectorEmitsObservedMetric(t *testing.T) {
	c := NewCollector()
	reg := prometheus.NewPedanticRegistry()
	require.NoError(t, reg.Register(c))

	c.Observe("smartmeter", "1234", []mapping.Snapshot{
		{Name: "active_power", Type: mapping.Gauge, Labels: map[string]string{"phase": "l2"}, Value: -692.9},
	}, map[string]string{"meter": "grid"})

	mfs, err := reg.Gather()
	require.NoError(t, err)

	var found bool
	for _, mf := range mfs {
		if mf.GetName() != "smartmeter_active_power" {
			continue
		}
		found = true
		m := mf.GetMetric()[0]
		require.InDelta(t, -692.9, m.GetGauge().GetValue(), 0.001)
		labels := map[string]string{}
		for _, l := range m.GetLabel() {
			labels[l.GetName()] = l.GetValue()
		}
		require.Equal(t, "grid", labels["meter"])
		require.Equal(t, "l2", labels["phase"])
	}
	require.True(t, found, "smartmeter_active_power not gathered")
}

func TestCollectorExpiresStaleMetrics(t *testing.T) {
	c := NewCollector()
	// Note: go-cache's Set treats a duration of -1 as cache.NoExpiration (never
	// expires), not "already expired". To actually exercise expiry we use a
	// tiny positive TTL and let it elapse.
	c.cache.Set("stale|smartmeter_x|", prometheus.MustNewConstMetric(
		prometheus.NewDesc("smartmeter_x", "", nil, nil), prometheus.GaugeValue, 1), time.Nanosecond)
	time.Sleep(5 * time.Millisecond)

	reg := prometheus.NewPedanticRegistry()
	_ = reg.Register(c)
	mfs, _ := reg.Gather()
	for _, mf := range mfs {
		require.NotEqual(t, "smartmeter_x", mf.GetName())
	}
}

// Copyright © 2026 Christian Fritz <mail@chr-fritz.de>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gatherFreshness registers a freshness collector backed by reads on a fresh registry and
// returns the samples it produces, keyed by their serial label.
func gatherFreshness(t *testing.T, reads map[uint32]time.Time) (name string, samples map[string]float64) {
	t.Helper()

	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(newFreshnessCollector(func() map[uint32]time.Time { return reads })))

	families, err := registry.Gather()
	require.NoError(t, err)
	if len(families) == 0 {
		return "", map[string]float64{}
	}
	require.Len(t, families, 1, "the collector must expose exactly one metric family")

	samples = make(map[string]float64)
	for _, m := range families[0].GetMetric() {
		require.Len(t, m.GetLabel(), 1)
		require.Equal(t, "serial", m.GetLabel()[0].GetName())
		samples[m.GetLabel()[0].GetValue()] = m.GetGauge().GetValue()
	}
	return families[0].GetName(), samples
}

// TestFreshnessCollectorReportsLastReadTime covers the signal that would have made the
// 12-hour silent outage visible: while the exporter was wedged it still served /metrics and
// still passed its health check, so nothing indicated that no device had been read for hours.
func TestFreshnessCollectorReportsLastReadTime(t *testing.T) {
	name, samples := gatherFreshness(t, map[uint32]time.Time{
		1234567890: time.Unix(1786434159, 0),
	})

	assert.Equal(t, "speedwire_last_successful_read_timestamp_seconds", name)
	assert.Equal(t, map[string]float64{"1234567890": 1786434159}, samples)
}

// TestFreshnessCollectorReportsEveryDeviceSeparately is the point of the serial label: with a
// grid meter and an inverter configured, a single global timestamp would stay fresh while one
// of the two is dead.
func TestFreshnessCollectorReportsEveryDeviceSeparately(t *testing.T) {
	_, samples := gatherFreshness(t, map[uint32]time.Time{
		1: time.Unix(1786434159, 0),
		2: time.Unix(1786400000, 0),
	})

	assert.Equal(t, map[string]float64{"1": 1786434159, "2": 1786400000}, samples)
}

// TestFreshnessCollectorReportsZeroForDevicesThatNeverAnswered keeps the worst case - a
// configured device that never came up at all - visible. Omitting the series instead would
// make it indistinguishable from a device that is not configured, and no `time() - gauge`
// alert can fire on a series that does not exist. 0 is the sentinel; see the README for the
// matching alert expression.
func TestFreshnessCollectorReportsZeroForDevicesThatNeverAnswered(t *testing.T) {
	_, samples := gatherFreshness(t, map[uint32]time.Time{
		1: time.Unix(1786434159, 0),
		2: {},
	})

	assert.Equal(t, map[string]float64{"1": 1786434159, "2": 0}, samples)
}

func TestFreshnessCollectorReportsSubSecondPrecision(t *testing.T) {
	_, samples := gatherFreshness(t, map[uint32]time.Time{
		1: time.Unix(1786434159, 500_000_000),
	})

	assert.InDelta(t, 1786434159.5, samples["1"], 1e-6)
}

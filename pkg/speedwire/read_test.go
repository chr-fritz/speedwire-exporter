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

package speedwire

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chr-fritz/speedwire-exporter/pkg/collector"
	"github.com/chr-fritz/speedwire-exporter/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/bboehmke/sunny"
)

// stalledMeter mimics an energy meter whose multicast stream has stopped: sunny's
// GetValuesCtx for energy meters has no deadline of its own, so it only ever returns when
// the context it was handed is done.
type stalledMeter struct{}

func (stalledMeter) IsEnergyMeter() bool { return true }

func (stalledMeter) GetValuesCtx(ctx context.Context) (map[sunny.ValueID]interface{}, error) {
	<-ctx.Done()
	return nil, errors.New("energy meter does not respond")
}

// respondingMeter returns a fixed value set immediately.
type respondingMeter struct{}

func (respondingMeter) IsEnergyMeter() bool { return true }

func (respondingMeter) GetValuesCtx(context.Context) (map[sunny.ValueID]interface{}, error) {
	return map[sunny.ValueID]interface{}{
		sunny.ActivePowerPlus:  float64(100),
		sunny.ActivePowerMinus: float64(0),
	}, nil
}

// emptyInverter reproduces sunny's inverter path: GetValuesCtx issues one request per value
// group, skips every group that errors, and returns a nil error regardless - so a device that
// answered nothing at all still looks like a successful read.
type emptyInverter struct{}

func (emptyInverter) IsEnergyMeter() bool { return false }

func (emptyInverter) GetValuesCtx(context.Context) (map[sunny.ValueID]interface{}, error) {
	return map[sunny.ValueID]interface{}{}, nil
}

func testListener(t *testing.T) *Listener {
	t.Helper()
	cfg := &config.Config{
		FetchInterval: 50 * time.Millisecond,
		Metrics:       config.MetricsConfig{EnergyMeterPrefix: "smartmeter", InverterPrefix: "sunny_inverter"},
		Devices:       []config.DeviceConfig{{Serial: 42, Labels: map[string]string{"meter": "grid"}}},
	}
	return NewListener(cfg, collector.NewCollector())
}

// TestReadReturnsWhenMeterStopsDelivering is the regression test for the outage where a
// single stalled read wedged the whole listener. read must bound the device read itself: it
// is handed the application-lifetime context, which never expires, so without its own
// deadline it blocks forever and the caller's event loop dies with it.
func TestReadReturnsWhenMeterStopsDelivering(t *testing.T) {
	l := testListener(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// context.Background() stands in for the application-lifetime context: it is never
		// cancelled, so only read's own deadline can end this call.
		l.read(context.Background(), 42, stalledMeter{})
	}()

	select {
	case <-done:
	case <-time.After(readTimeout + 2*time.Second):
		t.Fatal("read blocked on a stalled meter instead of timing out")
	}
}

// TestReadRecordsLastSuccessfulRead covers the freshness signal that makes a silent stall
// visible from the outside.
func TestReadRecordsLastSuccessfulRead(t *testing.T) {
	l := testListener(t)
	require.True(t, l.LastSuccessfulReads()[42].IsZero(), "no read happened yet")

	l.read(context.Background(), 42, respondingMeter{})

	assert.False(t, l.LastSuccessfulReads()[42].IsZero(), "a successful read must record its time")
}

func TestReadDoesNotRecordFreshnessOnFailure(t *testing.T) {
	l := testListener(t)

	l.read(context.Background(), 42, stalledMeter{})

	assert.True(t, l.LastSuccessfulReads()[42].IsZero(), "a failed read must not count as fresh data")
}

// TestReadDoesNotRecordFreshnessOnEmptyValues closes the gap sunny leaves on the inverter
// path: a read that returns no values at all is not a successful read, and must not keep the
// freshness signal alive while the device is in fact silent.
func TestReadDoesNotRecordFreshnessOnEmptyValues(t *testing.T) {
	l := testListener(t)

	l.read(context.Background(), 42, emptyInverter{})

	assert.True(t, l.LastSuccessfulReads()[42].IsZero(),
		"a read that produced no values must not count as fresh data")
}

// TestReadSkipsUnconfiguredDevice guards the existing early return: an unconfigured serial
// must not be read at all, so it can neither block nor look like fresh data.
func TestReadSkipsUnconfiguredDevice(t *testing.T) {
	l := testListener(t)

	l.read(context.Background(), 99, stalledMeter{})

	assert.NotContains(t, l.LastSuccessfulReads(), uint32(99))
}

// TestLastSuccessfulReadsCoversEveryConfiguredDevice keeps a device that has never answered
// visible. Reporting only devices that were read at least once would make the worst case -
// a device that never came up at all - indistinguishable from one that is not configured.
func TestLastSuccessfulReadsCoversEveryConfiguredDevice(t *testing.T) {
	l := testListener(t)
	l.cfg.Devices = append(l.cfg.Devices, config.DeviceConfig{Serial: 7, Labels: map[string]string{"meter": "pv"}})

	l.read(context.Background(), 42, respondingMeter{})
	reads := l.LastSuccessfulReads()

	require.Len(t, reads, 2)
	assert.False(t, reads[42].IsZero())
	assert.True(t, reads[7].IsZero(), "a configured device that never answered must still be reported")
}

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
	"sync"
	"testing"
	"time"

	"github.com/chr-fritz/speedwire-exporter/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/bboehmke/sunny"
)

// rendezvousMeter answers only once every other meter in its group has also started its read.
// Sequential reads therefore deadlock on the first device, which is what makes this a
// deterministic concurrency test rather than a timing one.
type rendezvousMeter struct{ barrier *sync.WaitGroup }

func (rendezvousMeter) IsEnergyMeter() bool { return true }
func (rendezvousMeter) Close()              {}

func (m rendezvousMeter) GetValuesCtx(context.Context) (map[sunny.ValueID]interface{}, error) {
	m.barrier.Done()
	m.barrier.Wait()
	return map[sunny.ValueID]interface{}{
		sunny.ActivePowerPlus:  float64(100),
		sunny.ActivePowerMinus: float64(0),
	}, nil
}

// stalledKnownMeter never answers, so it burns the full read deadline.
type stalledKnownMeter struct{ stalledMeter }

func (stalledKnownMeter) Close() {}

type respondingKnownMeter struct{ respondingMeter }

func (respondingKnownMeter) Close() {}

func listenerWithDevices(t *testing.T, serials ...uint32) *Listener {
	t.Helper()
	l := testListener(t)
	l.cfg.Devices = nil
	for _, s := range serials {
		l.cfg.Devices = append(l.cfg.Devices, config.DeviceConfig{
			Serial: s,
			Labels: map[string]string{"meter": "test"},
		})
	}
	return l
}

// TestReadAllReadsDevicesConcurrently is the fix for one slow device setting the pace for
// every other one. Reads used to run one after another inside the ticker branch, so a tick
// cost the sum of all read durations instead of the longest single one.
func TestReadAllReadsDevicesConcurrently(t *testing.T) {
	const devices = 3
	l := listenerWithDevices(t, 1, 2, 3)

	var barrier sync.WaitGroup
	barrier.Add(devices)
	known := map[uint32]knownDevice{}
	for serial := uint32(1); serial <= devices; serial++ {
		known[serial] = rendezvousMeter{barrier: &barrier}
	}

	done := make(chan struct{})
	go func() { defer close(done); l.readAll(context.Background(), known) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readAll never returned: the devices are read one after another, so the first read blocks the rest")
	}

	reads := l.LastSuccessfulReads()
	require.Len(t, reads, devices)
	for serial, last := range reads {
		assert.False(t, last.IsZero(), "device %d was never read", serial)
	}
}

// TestReadAllCostsOneDeadlineNotTheirSum is the concrete production case: an energy meter and
// an inverter are configured and the inverter stops answering. A tick must cost one read
// deadline, not one per unresponsive device - once the total exceeds the fetch interval, ticks
// are dropped and the healthy device is sampled at a fraction of the configured rate.
//
// Two stalled devices rather than one on purpose: with a single one, sequential reads would
// pass or fail depending on the random map iteration order. Two always cost two deadlines
// sequentially and one concurrently, whichever order they run in.
func TestReadAllCostsOneDeadlineNotTheirSum(t *testing.T) {
	l := listenerWithDevices(t, 1, 2, 3)
	known := map[uint32]knownDevice{
		1: stalledKnownMeter{},
		2: stalledKnownMeter{},
		3: respondingKnownMeter{},
	}

	start := time.Now()
	l.readAll(context.Background(), known)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, readTimeout, "sanity: the stalled devices did hit their deadline")
	assert.Less(t, elapsed, 2*readTimeout, "each stalled device added its own deadline to the tick")
	assert.False(t, l.LastSuccessfulReads()[3].IsZero(), "the responding device must still have been read")
	assert.True(t, l.LastSuccessfulReads()[1].IsZero(), "a stalled device must not count as read")
}

func TestReadAllHandlesNoDevices(t *testing.T) {
	l := listenerWithDevices(t)

	assert.NotPanics(t, func() { l.readAll(context.Background(), map[uint32]knownDevice{}) })
}

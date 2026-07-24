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
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/chr-fritz/speedwire-exporter/pkg/collector"
	"github.com/chr-fritz/speedwire-exporter/pkg/config"
	"github.com/chr-fritz/speedwire-exporter/pkg/mapping"
	"gitlab.com/bboehmke/sunny"
)

// Listener discovers Speedwire devices and periodically feeds their values into a Collector.
type Listener struct {
	cfg          *config.Config
	col          *collector.Collector
	seenUnmapped sync.Map // sunny.ValueID -> struct{}
}

// discoveryWindow bounds a single discovery cycle, mirroring Discover and sunny's own
// SimpleDiscoverDevices idiom. discoveryInterval is how often the device list is refreshed.
//
// These are package variables (not constants) only so tests can shorten them.
var (
	discoveryWindow   = 3 * time.Second
	discoveryInterval = 5 * time.Minute
)

// NewListener creates a Listener that reads configured devices on cfg.FetchInterval and observes
// their values via col.
func NewListener(cfg *config.Config, col *collector.Collector) *Listener {
	return &Listener{cfg: cfg, col: col}
}

// Run periodically discovers Speedwire devices on the configured interface and, on every
// FetchInterval tick, reads each known device and feeds configured ones into the collector. It
// blocks until ctx is done.
func (l *Listener) Run(ctx context.Context) {
	conn, err := sunny.NewConnection(l.cfg.Interface)
	if err != nil {
		slog.With("err", err).Error("can not open speedwire connection")
		return
	}

	devs := make(chan *sunny.Device, 10)
	go l.discoverLoop(ctx, conn, devs)

	ticker := time.NewTicker(l.cfg.FetchInterval)
	defer ticker.Stop()
	known := map[uint32]*sunny.Device{}

	for {
		select {
		case dev := <-devs:
			// discoverLoop closes devs on shutdown, yielding a nil dev; skip it so we never
			// insert a nil device into known.
			if dev == nil {
				continue
			}
			serial := dev.SerialNumber()
			// Re-discovery creates a fresh *sunny.Device (with a new receiver channel
			// registered on the shared, long-lived connection) for a device we may already
			// know. Close the previous one so its receiver registration does not leak.
			if old, ok := known[serial]; ok {
				old.Close()
			}
			known[serial] = dev
		case <-ticker.C:
			for serial, dev := range known {
				l.read(ctx, serial, dev)
			}
		case <-ctx.Done():
			// Drain devs until discoverLoop closes it. A discovery window in flight may have
			// per-IP goroutines blocked on their `devs <- device` send; DiscoverDevices only
			// returns (via wg.Wait()) once those sends are received. Draining unblocks them so
			// discoverLoop can finish and close(devs).
			for range devs {
			}
			return
		}
	}
}

// discoverLoop repeatedly runs a bounded discovery cycle, feeding discovered devices into devs,
// until ctx is done. Discovery must NOT run continuously: sunny's DiscoverDevices spawns a
// goroutine for every received packet and re-attempts NewDevice on every packet from any source
// whose handshake never completes (for example this host's own multicast discovery requests
// looped back, or any other Speedwire-speaking device that is not a readable inverter/energy
// meter). Each such attempt blocks for ~3s holding an internal mutex, so packets arriving faster
// than one per 3s pile up goroutines without bound. Time-boxing each cycle to discoveryWindow
// makes DiscoverDevices return (draining its workers) instead of running forever, which keeps the
// goroutine count bounded.
func (l *Listener) discoverLoop(ctx context.Context, conn *sunny.Connection, devs chan *sunny.Device) {
	defer close(devs)
	for {
		discoverCtx, cancel := context.WithTimeout(ctx, discoveryWindow)
		conn.DiscoverDevices(discoverCtx, devs, l.cfg.Discovery.Password)
		cancel()

		select {
		case <-ctx.Done():
			return
		case <-time.After(discoveryInterval):
		}
	}
}

func (l *Listener) read(ctx context.Context, serial uint32, dev *sunny.Device) {
	labels, ok := l.cfg.LabelsFor(serial)
	if !ok {
		slog.With("serial", serial).Debug("skipping unconfigured device")
		return
	}
	values, err := dev.GetValuesCtx(ctx)
	if err != nil {
		slog.With("serial", serial, "err", err).Warn("can not read values")
		return
	}
	serialStr := strconv.FormatUint(uint64(serial), 10)

	isEM := dev.IsEnergyMeter()
	l.logUnmapped(values, mappedPredicate(isEM))
	for _, o := range deviceObservations(isEM, values, l.cfg.Metrics) {
		l.col.Observe(o.prefix, serialStr, o.snaps, labels)
	}
}

// observation is a single pending Collector.Observe call: the metric prefix and the
// snapshots to emit under it.
type observation struct {
	prefix string
	snaps  []mapping.Snapshot
}

// mappedPredicate returns the "is this ValueID mapped" predicate for the given device kind.
func mappedPredicate(isEnergyMeter bool) func(sunny.ValueID) bool {
	if isEnergyMeter {
		return mapping.IsMappedEnergyMeter
	}
	return mapping.IsMappedInverter
}

// deviceObservations computes the observations read should emit for a device's raw values,
// depending on its kind (energy meter vs. inverter) and whether info series are enabled.
func deviceObservations(isEnergyMeter bool, values map[sunny.ValueID]interface{}, m config.MetricsConfig) []observation {
	if isEnergyMeter {
		out := []observation{{m.EnergyMeterPrefix, mapping.MapEnergyMeter(values)}}
		if m.Info {
			if info, ok := mapping.EnergyMeterInfo(values); ok {
				out = append(out, observation{m.EnergyMeterPrefix, []mapping.Snapshot{info}})
			}
		}
		return out
	}

	out := []observation{{m.InverterPrefix, mapping.MapInverter(values)}}
	if m.Info {
		if info, ok := mapping.InverterInfo(values); ok {
			out = append(out, observation{m.InverterPrefix, []mapping.Snapshot{info}})
		}
	}
	return out
}

// logUnmapped logs values the given predicate does not cover, once per ValueID at info level.
func (l *Listener) logUnmapped(values map[sunny.ValueID]interface{}, isMapped func(sunny.ValueID) bool) {
	for id := range values {
		if !isMapped(id) {
			if _, loaded := l.seenUnmapped.LoadOrStore(id, struct{}{}); !loaded {
				slog.With("valueId", id, "description", sunny.GetValueInfo(id).Description).
					Info("unmapped speedwire value")
			}
		}
	}
}

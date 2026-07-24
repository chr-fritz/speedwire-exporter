// Copyright © 2020-2022 Christian Fritz <mail@chr-fritz.de>
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

// NewListener creates a Listener that reads configured devices on cfg.FetchInterval and observes
// their values via col.
func NewListener(cfg *config.Config, col *collector.Collector) *Listener {
	return &Listener{cfg: cfg, col: col}
}

// Run discovers Speedwire devices on the configured interface and, on every FetchInterval tick,
// reads each known device and feeds configured ones into the collector. It blocks until ctx is
// done.
func (l *Listener) Run(ctx context.Context) {
	conn, err := sunny.NewConnection(l.cfg.Interface)
	if err != nil {
		slog.With("err", err).Error("can not open speedwire connection")
		return
	}

	devs := make(chan *sunny.Device, 10)
	go func() {
		conn.DiscoverDevices(ctx, devs, l.cfg.Discovery.Password)
		close(devs)
	}()

	ticker := time.NewTicker(l.cfg.FetchInterval)
	defer ticker.Stop()
	known := map[uint32]*sunny.Device{}

	for {
		select {
		case dev := <-devs:
			// A closed channel yields a nil dev once DiscoverDevices has returned; skip it so we
			// never insert a nil device into known.
			if dev != nil {
				known[dev.SerialNumber()] = dev
			}
		case <-ticker.C:
			for serial, dev := range known {
				l.read(ctx, serial, dev)
			}
		case <-ctx.Done():
			// Drain devs until DiscoverDevices closes it: DiscoverDevices only returns (and thus
			// only closes devs) after its own wg.Wait() completes, which requires every in-flight
			// per-IP goroutine's blocking `devices <- device` send to be received. If we stopped
			// reading here, those sends would block forever and leak the discovery goroutine
			// (and its discoverer registration on conn). Draining unblocks them so
			// DiscoverDevices can finish and the goroutine above can close(devs) and exit.
			for range devs {
			}
			return
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

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

	freshnessMu sync.RWMutex
	lastReads   map[uint32]time.Time // serial -> time of its last successful read
}

// meterReader is the part of *sunny.Device that read depends on. It exists so the read path
// can be exercised without a Speedwire network.
type meterReader interface {
	GetValuesCtx(ctx context.Context) (map[sunny.ValueID]interface{}, error)
	IsEnergyMeter() bool
}

// knownDevice is a device the listener holds on to between reads: it is read on every tick
// and released when re-discovery replaces it.
type knownDevice interface {
	meterReader
	Close()
}

// readTimeout bounds a single device read, mirroring the deadline sunny's own ctx-less
// Device.GetValues applies.
//
// It is deliberately not tied to FetchInterval. An energy meter read cannot be served from
// buffered data - sunny clears the receiver channel first and then waits for the next
// datagram - so a deadline near the meter's ~1s multicast period would turn healthy reads
// into coin flips. Overrunning a tick is harmless: time.Ticker drops missed ticks.
const readTimeout = 3 * time.Second

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

	ticker := time.NewTicker(l.fetchInterval())
	defer ticker.Stop()
	known := map[uint32]knownDevice{}

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
			l.readAll(ctx, known)
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

// fetchInterval, discoveryInterval and discoveryWindow read their setting from the
// configuration, falling back to the package default when it is not positive.
//
// Configurations loaded through the commands are defaulted and validated at startup, so in
// practice these always return the configured value. The fallbacks exist for Listeners built
// programmatically - a zero fetch interval panics time.NewTicker, and a zero discovery
// interval would make the loop rediscover without pause.
func (l *Listener) fetchInterval() time.Duration {
	if l.cfg.FetchInterval > 0 {
		return l.cfg.FetchInterval
	}
	return config.DefaultFetchInterval
}

func (l *Listener) discoveryInterval() time.Duration {
	if l.cfg.Discovery.Interval > 0 {
		return l.cfg.Discovery.Interval
	}
	return config.DefaultDiscoveryInterval
}

func (l *Listener) discoveryWindow() time.Duration {
	if l.cfg.Discovery.Window > 0 {
		return l.cfg.Discovery.Window
	}
	return config.DefaultDiscoveryWindow
}

// needsDiscovery reports whether a discovery cycle is worth running.
//
// Discovery is not free for the rest of the network: a cycle broadcasts an SMA discovery
// request to the whole Speedwire multicast group every 500ms for the duration of the window,
// and third-party Speedwire clients on that group have to cope with every one of them. It
// buys nothing while every configured device is already being read, because the device list
// cannot change: the exporter only ever reads the serials named in the configuration.
//
// So only look when there is something to find - a configured device that has never answered,
// or one that has gone silent for longer than a full discovery period and may have changed
// address or rebooted. With nothing configured at all there is nothing to find either, and
// staying quiet on the wire beats announcing ourselves for no reason.
func (l *Listener) needsDiscovery(now time.Time) bool {
	reads := l.LastSuccessfulReads()
	if len(reads) == 0 {
		return false
	}
	for _, last := range reads {
		if last.IsZero() || now.Sub(last) >= l.discoveryInterval() {
			return true
		}
	}
	return false
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
//
// Cycles are additionally skipped whenever needsDiscovery says there is nothing to look for,
// which is the steady state. That matters beyond this process: a cycle broadcasts an SMA
// discovery request to the shared multicast group every 500ms for the whole window, and other
// Speedwire clients on that group have to cope with each one.
func (l *Listener) discoverLoop(ctx context.Context, conn *sunny.Connection, devs chan *sunny.Device) {
	defer close(devs)
	for {
		if l.needsDiscovery(time.Now()) {
			discoverCtx, cancel := context.WithTimeout(ctx, l.discoveryWindow())
			conn.DiscoverDevices(discoverCtx, devs, l.cfg.Discovery.Password)
			cancel()
		} else {
			slog.Debug("skipping Speedwire discovery, every configured device is being read")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(l.discoveryInterval()):
		}
	}
}

// LastSuccessfulReads returns, for every configured device, when it was last read
// successfully. Devices that have never answered are included with the zero time, so a device
// that never came up at all is visible rather than simply missing. It backs the
// speedwire_last_successful_read_timestamp_seconds metric.
func (l *Listener) LastSuccessfulReads() map[uint32]time.Time {
	l.freshnessMu.RLock()
	defer l.freshnessMu.RUnlock()

	reads := make(map[uint32]time.Time, len(l.cfg.Devices))
	for _, d := range l.cfg.Devices {
		reads[d.Serial] = l.lastReads[d.Serial]
	}
	return reads
}

// readAll reads every known device once, concurrently, and returns when all of them are done.
//
// Concurrency is what keeps one unresponsive device from setting the pace for the rest: reads
// are bounded by readTimeout, so read them one after another and a tick costs the sum of those
// deadlines instead of the longest single one. With an energy meter and an inverter
// configured, a dead inverter would otherwise spend 3s of every tick before the meter is read
// at all - and once the total exceeds the fetch interval, ticks start being dropped and the
// healthy device is sampled at a fraction of the configured rate.
//
// Waiting for all of them before returning is deliberate: it keeps a slow round from
// overlapping the next one, exactly as the sequential version did. time.Ticker drops the ticks
// that fall in between.
//
// Concurrent reads of *distinct* devices are safe: each holds its own receiver channel, the
// shared UDP socket tolerates concurrent writes, and the collector, the unmapped-value set and
// the freshness map are all synchronised.
func (l *Listener) readAll(ctx context.Context, known map[uint32]knownDevice) {
	var wg sync.WaitGroup
	for serial, dev := range known {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.read(ctx, serial, dev)
		}()
	}
	wg.Wait()
}

func (l *Listener) read(ctx context.Context, serial uint32, dev meterReader) {
	labels, ok := l.cfg.LabelsFor(serial)
	if !ok {
		slog.With("serial", serial).Debug("skipping unconfigured device")
		return
	}
	// The read MUST have its own deadline. ctx lives as long as the process, and sunny's
	// GetValuesCtx has no deadline of its own for energy meters: it blocks on the device's
	// receiver channel until the context it was handed is done. Passing ctx straight through
	// therefore turns one missed multicast datagram into a permanent hang of Run's single
	// event loop - no further reads, no discovery handling, no log output, until restart.
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	values, err := dev.GetValuesCtx(readCtx)
	if err != nil {
		slog.With("serial", serial, "err", err).Warn("can not read values")
		return
	}
	if len(values) == 0 {
		// sunny's inverter path collects one request group at a time, skips every group that
		// errors and returns a nil error regardless - so "no values at all" arrives here as a
		// success. Counting it as one would keep the freshness signal alive for a device that
		// is in fact answering nothing.
		slog.With("serial", serial).Warn("device returned no values")
		return
	}
	l.recordSuccessfulRead(serial)
	serialStr := strconv.FormatUint(uint64(serial), 10)

	isEM := dev.IsEnergyMeter()
	l.logUnmapped(values, mappedPredicate(isEM))
	for _, o := range deviceObservations(isEM, values, l.cfg.Metrics) {
		l.col.Observe(o.prefix, serialStr, o.snaps, labels)
	}
}

// recordSuccessfulRead marks now as the last time the given device answered with values.
func (l *Listener) recordSuccessfulRead(serial uint32) {
	l.freshnessMu.Lock()
	defer l.freshnessMu.Unlock()
	if l.lastReads == nil {
		l.lastReads = make(map[uint32]time.Time)
	}
	l.lastReads[serial] = time.Now()
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

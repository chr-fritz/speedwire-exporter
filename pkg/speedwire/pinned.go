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
	"time"

	"gitlab.com/bboehmke/sunny"
)

// acquiredDevice is a device handle the listener has just obtained, from either acquisition
// path - dialled at a configured address, or found by discovery.
type acquiredDevice interface {
	knownDevice
	SerialNumber() uint32
}

// deviceDialer opens the Speedwire device at address. Connection.NewDevice is the real one;
// the indirection keeps the pinned-address path testable without a Speedwire network.
type deviceDialer func(address, password string) (acquiredDevice, error)

// dialer adapts a sunny Connection to deviceDialer.
func dialer(conn *sunny.Connection) deviceDialer {
	return func(address, password string) (acquiredDevice, error) {
		return conn.NewDevice(address, password)
	}
}

// dialPinned opens every configured device that has an address and is not currently being read,
// and hands the resulting handles to devs.
//
// This is the whole point of pinning an address: opening a device this way is a unicast
// handshake with one host, where discovery is a burst of broadcasts to the Speedwire multicast
// group that every other client on the segment has to parse. A configuration in which every
// device is pinned never sends a discovery request at all - not even at startup.
//
// A device that is already being read successfully is skipped; re-dialling it would register a
// second receiver channel for it on the shared connection every cycle.
func (l *Listener) dialPinned(ctx context.Context, dial deviceDialer, devs chan<- acquiredDevice) {
	now := time.Now()
	reads := l.LastSuccessfulReads()

	for _, cfg := range l.cfg.Devices {
		// sunny builds NewDevice's 3s handshake deadline from context.Background(), so an
		// unreachable host cannot be cut short once dialling has started. Check between devices
		// instead, or shutdown waits 3s for every pinned device that is down.
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !cfg.IsPinned() || !l.isStale(reads[cfg.Serial], now) {
			continue
		}

		dev, err := dial(cfg.Address, l.cfg.Discovery.Password)
		if err != nil {
			slog.With("serial", cfg.Serial, "address", cfg.Address, "err", err).
				Warn("can not reach configured device address")
			continue
		}

		// Pinning trades discovery for trust in the configuration, so verify what answered.
		// If the address has moved to another device - a lost DHCP reservation, a typo - reading
		// it would publish that device's values under these labels.
		if dev.SerialNumber() != cfg.Serial {
			slog.With("configuredSerial", cfg.Serial, "actualSerial", dev.SerialNumber(), "address", cfg.Address).
				Warn("device at configured address reports a different serial")
			dev.Close()
			continue
		}

		select {
		case devs <- dev:
		case <-ctx.Done():
			dev.Close()
			return
		}
	}
}

// isStale reports whether a device's last successful read is too old to consider it working.
func (l *Listener) isStale(last time.Time, now time.Time) bool {
	return last.IsZero() || now.Sub(last) >= l.discoveryInterval()
}

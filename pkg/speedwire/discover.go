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
	"net"
	"time"

	"gitlab.com/bboehmke/sunny"
)

// DiscoveredDevice holds the identity and current values of a Speedwire device found during discovery.
type DiscoveredDevice struct {
	Serial        uint32
	Address       string
	IsEnergyMeter bool
	Values        map[sunny.ValueID]interface{}
}

// discoveredReader is the part of *sunny.Device that readDiscovered depends on. It exists so
// the readout path can be exercised without a Speedwire network.
type discoveredReader interface {
	meterReader
	SerialNumber() uint32
	Address() *net.UDPAddr
	Close()
}

// readDiscovered reads dev's current values under its own deadline and releases the receiver
// channel the device registered on the shared, process-lifetime connection in NewDevice.
//
// The deadline is not optional: sunny's GetValuesCtx has none of its own for energy meters,
// so handing it the caller's context would let one unresponsive device consume the entire
// budget of whoever called Discover - the /devices handler's 10s, or readout's. A device that
// does not answer in time is still reported, just without values.
func readDiscovered(ctx context.Context, dev discoveredReader, timeout time.Duration) DiscoveredDevice {
	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	values, err := dev.GetValuesCtx(readCtx)
	if err != nil {
		slog.Warn("failed to read values from device", "serial", dev.SerialNumber(), "err", err)
	}
	dev.Close()

	return DiscoveredDevice{
		Serial:        dev.SerialNumber(),
		Address:       dev.Address().String(),
		IsEnergyMeter: dev.IsEnergyMeter(),
		Values:        values,
	}
}

// Discover finds all Speedwire devices on the given interface and reads their current values once.
//
// Discovery itself is bounded to 3 seconds (mirroring sunny's own
// Connection.SimpleDiscoverDevices idiom): a derived context with a 3s deadline is passed to
// conn.DiscoverDevices, which blocks until that deadline (or the parent ctx) is done, drains its
// internal worker goroutines via wg.Wait() and unregisters its discoverer before returning. Only
// after DiscoverDevices has returned do we close the results channel and wait for the drain
// goroutine below to finish, so no producer can ever block on a send and no discoverer
// registration is leaked.
func Discover(ctx context.Context, iface, password string) ([]DiscoveredDevice, error) {
	conn, err := sunny.NewConnection(iface)
	if err != nil {
		return nil, err
	}

	discoverCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	devs := make(chan *sunny.Device, 10)
	var result []DiscoveredDevice
	done := make(chan struct{})
	go func() {
		defer close(done)
		for dev := range devs {
			result = append(result, readDiscovered(ctx, dev, readTimeout))
		}
	}()

	// DiscoverDevices blocks until discoverCtx is done, then drains its own worker goroutines
	// (wg.Wait()) and unregisters its discoverer before returning.
	conn.DiscoverDevices(discoverCtx, devs, password)
	close(devs)
	<-done

	if err := ctx.Err(); err != nil {
		return result, err
	}
	return result, nil
}

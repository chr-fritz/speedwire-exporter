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

// DiscoveredDevice holds the identity and current values of a Speedwire device found during discovery.
type DiscoveredDevice struct {
	Serial        uint32
	Address       string
	IsEnergyMeter bool
	Values        map[sunny.ValueID]interface{}
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
			values, err := dev.GetValuesCtx(ctx)
			if err != nil {
				slog.Warn("failed to read values from device", "serial", dev.SerialNumber(), "err", err)
			}
			result = append(result, DiscoveredDevice{
				Serial:        dev.SerialNumber(),
				Address:       dev.Address().String(),
				IsEnergyMeter: dev.IsEnergyMeter(),
				Values:        values,
			})
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

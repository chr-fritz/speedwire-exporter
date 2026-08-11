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
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stalledDiscovered is a discovered device that never answers, like an energy meter whose
// multicast stream has stopped between discovery and readout.
type stalledDiscovered struct {
	stalledMeter
	closed bool
}

func (d *stalledDiscovered) SerialNumber() uint32 { return 42 }
func (d *stalledDiscovered) Address() *net.UDPAddr {
	return &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 9522}
}
func (d *stalledDiscovered) Close() { d.closed = true }

// respondingDiscovered answers immediately.
type respondingDiscovered struct {
	respondingMeter
	closed bool
}

func (d *respondingDiscovered) SerialNumber() uint32 { return 7 }
func (d *respondingDiscovered) Address() *net.UDPAddr {
	return &net.UDPAddr{IP: net.IPv4(10, 0, 0, 2), Port: 9522}
}
func (d *respondingDiscovered) Close() { d.closed = true }

// TestReadDiscoveredReturnsWhenDeviceStalls covers the /devices variant of the wedge:
// Discover passed the caller's context straight to GetValuesCtx, so one unresponsive device
// consumed the handler's whole 10s budget and starved every device behind it.
func TestReadDiscoveredReturnsWhenDeviceStalls(t *testing.T) {
	dev := &stalledDiscovered{}

	done := make(chan DiscoveredDevice, 1)
	go func() { done <- readDiscovered(context.Background(), dev, 100*time.Millisecond) }()

	select {
	case got := <-done:
		assert.Equal(t, uint32(42), got.Serial)
		assert.Equal(t, "10.0.0.1:9522", got.Address)
		assert.Nil(t, got.Values, "a device that never answered has no values")
	case <-time.After(2 * time.Second):
		t.Fatal("readDiscovered blocked on a stalled device instead of timing out")
	}
}

// TestReadDiscoveredAlwaysReleasesReceiver guards the receiver registration each discovered
// device holds on the shared, process-lifetime connection.
func TestReadDiscoveredAlwaysReleasesReceiver(t *testing.T) {
	stalled := &stalledDiscovered{}
	readDiscovered(context.Background(), stalled, 50*time.Millisecond)
	assert.True(t, stalled.closed, "a device that timed out must still be closed")

	responding := &respondingDiscovered{}
	readDiscovered(context.Background(), responding, time.Second)
	assert.True(t, responding.closed)
}

func TestReadDiscoveredReturnsValuesOfRespondingDevice(t *testing.T) {
	got := readDiscovered(context.Background(), &respondingDiscovered{}, time.Second)

	require.NotNil(t, got.Values)
	assert.Equal(t, uint32(7), got.Serial)
	assert.True(t, got.IsEnergyMeter)
}

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
	"sync/atomic"
	"testing"
	"time"

	"github.com/chr-fritz/speedwire-exporter/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDevice is an acquiredDevice that records whether it was released.
type fakeDevice struct {
	respondingMeter
	serial uint32
	closed bool
}

func (d *fakeDevice) SerialNumber() uint32 { return d.serial }
func (d *fakeDevice) Close()               { d.closed = true }

// recordingDialer stands in for Connection.NewDevice.
type recordingDialer struct {
	dialled []string
	devices map[string]*fakeDevice
	err     error
}

func (d *recordingDialer) dial(address, _ string) (acquiredDevice, error) {
	d.dialled = append(d.dialled, address)
	if d.err != nil {
		return nil, d.err
	}
	dev, ok := d.devices[address]
	if !ok {
		// Not returning the zero value: that is a typed nil in a non-nil interface, which
		// surfaces as an opaque panic instead of a readable failure.
		return nil, errors.New("no device registered at " + address)
	}
	return dev, nil
}

func pinnedListener(t *testing.T, devices ...config.DeviceConfig) *Listener {
	t.Helper()
	l := testListener(t)
	l.cfg.Devices = devices
	return l
}

func drain(devs chan acquiredDevice) []acquiredDevice {
	close(devs)
	var got []acquiredDevice
	for d := range devs {
		got = append(got, d)
	}
	return got
}

// TestDialPinnedOpensConfiguredAddressesDirectly is the point of the feature: a device with a
// known address is opened with a single unicast handshake instead of a multicast broadcast
// that every other Speedwire client on the segment has to cope with.
func TestDialPinnedOpensConfiguredAddressesDirectly(t *testing.T) {
	l := pinnedListener(t, config.DeviceConfig{Serial: 42, Address: "10.0.0.5"})
	dialer := &recordingDialer{devices: map[string]*fakeDevice{"10.0.0.5": {serial: 42}}}
	devs := make(chan acquiredDevice, 4)

	l.dialPinned(context.Background(), dialer.dial, devs)

	assert.Equal(t, []string{"10.0.0.5"}, dialer.dialled)
	got := drain(devs)
	require.Len(t, got, 1)
	assert.Equal(t, uint32(42), got[0].SerialNumber())
}

func TestDialPinnedIgnoresDevicesWithoutAddress(t *testing.T) {
	l := pinnedListener(t, config.DeviceConfig{Serial: 42})
	dialer := &recordingDialer{}
	devs := make(chan acquiredDevice, 4)

	l.dialPinned(context.Background(), dialer.dial, devs)

	assert.Empty(t, dialer.dialled, "an unpinned device is found by discovery, not dialled")
	assert.Empty(t, drain(devs))
}

// TestDialPinnedSkipsDevicesThatAreBeingRead keeps the handshake from being repeated for a
// device that already has a working handle; re-dialling would register a second receiver
// channel for it on every cycle.
func TestDialPinnedSkipsDevicesThatAreBeingRead(t *testing.T) {
	l := pinnedListener(t, config.DeviceConfig{Serial: 42, Address: "10.0.0.5", Labels: map[string]string{"m": "g"}})
	l.read(context.Background(), 42, respondingMeter{})

	dialer := &recordingDialer{devices: map[string]*fakeDevice{"10.0.0.5": {serial: 42}}}
	devs := make(chan acquiredDevice, 4)

	l.dialPinned(context.Background(), dialer.dial, devs)

	assert.Empty(t, dialer.dialled)
	assert.Empty(t, drain(devs))
}

// TestDialPinnedRejectsSerialMismatch guards the failure mode that pinning introduces: if the
// address ends up belonging to a different device - a lost DHCP reservation, a typo - reading
// it would silently publish another device's values under the configured labels.
func TestDialPinnedRejectsSerialMismatch(t *testing.T) {
	l := pinnedListener(t, config.DeviceConfig{Serial: 42, Address: "10.0.0.5"})
	wrong := &fakeDevice{serial: 99}
	dialer := &recordingDialer{devices: map[string]*fakeDevice{"10.0.0.5": wrong}}
	devs := make(chan acquiredDevice, 4)

	l.dialPinned(context.Background(), dialer.dial, devs)

	assert.Empty(t, drain(devs), "a device with the wrong serial must not be used")
	assert.True(t, wrong.closed, "and must be released so its receiver registration does not leak")
}

func TestDialPinnedSurvivesAnUnreachableDevice(t *testing.T) {
	l := pinnedListener(t,
		config.DeviceConfig{Serial: 1, Address: "10.0.0.5"},
		config.DeviceConfig{Serial: 2, Address: "10.0.0.6"},
	)
	dialer := &recordingDialer{err: errors.New("no Speedwire ping response")}
	devs := make(chan acquiredDevice, 4)

	l.dialPinned(context.Background(), dialer.dial, devs)

	assert.Equal(t, []string{"10.0.0.5", "10.0.0.6"}, dialer.dialled,
		"one unreachable device must not stop the others from being dialled")
	assert.Empty(t, drain(devs))
}

// TestNeedsDiscoveryIgnoresPinnedDevices is what actually removes the startup broadcast: a
// pinned device is never a reason to go looking on the multicast group.
//
// The second case records a deliberate decision. A pinned device that cannot be reached is
// retried at its address and reported through the freshness metric and a warning, but it never
// causes a fallback to discovery: falling back would quietly restore the broadcast traffic
// pinning exists to avoid, and hide the configuration error causing it.
func TestNeedsDiscoveryIgnoresPinnedDevices(t *testing.T) {
	l := pinnedListener(t, config.DeviceConfig{Serial: 42, Address: "10.0.0.5"})

	assert.False(t, l.needsDiscovery(time.Now()), "not even before its first read")
	assert.False(t, l.needsDiscovery(time.Now().Add(24*time.Hour)), "and not after a day of silence")
}

// TestNeedsDiscoveryStillCoversUnpinnedDevices keeps mixed configurations working: pinning one
// device must not stop the exporter from looking for the others.
func TestNeedsDiscoveryStillCoversUnpinnedDevices(t *testing.T) {
	l := pinnedListener(t,
		config.DeviceConfig{Serial: 1, Address: "10.0.0.5"},
		config.DeviceConfig{Serial: 2},
	)

	assert.True(t, l.needsDiscovery(time.Now()))
}

// acquireProbe records which acquisition paths acquireLoop actually used.
type acquireProbe struct {
	dialled   chan string
	discovers atomic.Int64
}

func newAcquireProbe() *acquireProbe {
	return &acquireProbe{dialled: make(chan string, 64)}
}

func (p *acquireProbe) dial(address, _ string) (acquiredDevice, error) {
	select {
	case p.dialled <- address:
	default:
	}
	return nil, errors.New("not reachable in this test")
}

func (p *acquireProbe) discover(context.Context, chan<- acquiredDevice) {
	p.discovers.Add(1)
}

// runAcquireLoop spins the loop until probe reports the first activity, then shuts it down.
func runAcquireLoop(t *testing.T, l *Listener, probe *acquireProbe, sawActivity func() bool) {
	t.Helper()
	l.cfg.Discovery.Interval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	devs := make(chan acquiredDevice, 16)
	done := make(chan struct{})
	go func() { defer close(done); l.acquireLoop(ctx, probe.dial, probe.discover, devs) }()

	require.Eventually(t, sawActivity, 2*time.Second, 5*time.Millisecond,
		"acquireLoop never ran an acquisition cycle")
	cancel()
	<-done
}

// TestAcquireLoopNeverDiscoversForAFullyPinnedConfig is the feature's central promise, and it
// has to be asserted against the loop rather than against needsDiscovery alone: testing only
// the predicate leaves the wiring free to call discovery unconditionally.
func TestAcquireLoopNeverDiscoversForAFullyPinnedConfig(t *testing.T) {
	l := pinnedListener(t, config.DeviceConfig{Serial: 42, Address: "10.0.0.5"})
	probe := newAcquireProbe()

	runAcquireLoop(t, l, probe, func() bool { return len(probe.dialled) > 0 })

	assert.Zero(t, probe.discovers.Load(),
		"a fully pinned configuration must never broadcast a discovery request")
	assert.Equal(t, "10.0.0.5", <-probe.dialled)
}

// TestAcquireLoopDiscoversForUnpinnedDevices is the other half: pinning must not switch
// discovery off for devices that still depend on it.
func TestAcquireLoopDiscoversForUnpinnedDevices(t *testing.T) {
	l := pinnedListener(t,
		config.DeviceConfig{Serial: 1, Address: "10.0.0.5"},
		config.DeviceConfig{Serial: 2},
	)
	probe := newAcquireProbe()

	runAcquireLoop(t, l, probe, func() bool { return probe.discovers.Load() > 0 })

	assert.NotEmpty(t, probe.dialled, "the pinned device must still be dialled")
}

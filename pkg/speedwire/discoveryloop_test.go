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
	"testing"
	"time"

	"github.com/chr-fritz/speedwire-exporter/pkg/collector"
	"github.com/chr-fritz/speedwire-exporter/pkg/config"
	"github.com/stretchr/testify/assert"
)

// TestNeedsDiscoveryBeforeAnyDeviceAnswered keeps the startup case working: nothing can be
// read until discovery has found it, so the first cycle must always run.
func TestNeedsDiscoveryBeforeAnyDeviceAnswered(t *testing.T) {
	l := testListener(t)

	assert.True(t, l.needsDiscovery(time.Now()))
}

// TestNeedsDiscoveryIsSkippedWhileEveryDeviceIsFresh is the fix for the multicast noise: once
// every configured device is being read successfully, re-broadcasting discovery to the whole
// Speedwire group finds nothing new and only disturbs other devices on the group.
func TestNeedsDiscoveryIsSkippedWhileEveryDeviceIsFresh(t *testing.T) {
	l := testListener(t)
	l.read(context.Background(), 42, respondingMeter{})

	assert.False(t, l.needsDiscovery(time.Now()))
}

// TestNeedsDiscoveryWhenADeviceGoesSilent covers the case re-discovery actually exists for:
// a device that changed address or rebooted stops answering, and has to be found again.
func TestNeedsDiscoveryWhenADeviceGoesSilent(t *testing.T) {
	l := testListener(t)
	l.read(context.Background(), 42, respondingMeter{})

	assert.True(t, l.needsDiscovery(time.Now().Add(l.discoveryInterval()+time.Second)))
}

// TestNeedsDiscoveryWhenOneOfSeveralDevicesIsSilent guards against the obvious mistake of
// treating "something answered" as good enough.
func TestNeedsDiscoveryWhenOneOfSeveralDevicesIsSilent(t *testing.T) {
	l := testListener(t)
	l.cfg.Devices = append(l.cfg.Devices, config.DeviceConfig{Serial: 7, Labels: map[string]string{"meter": "pv"}})

	l.read(context.Background(), 42, respondingMeter{})

	assert.True(t, l.needsDiscovery(time.Now()), "serial 7 has never answered")
}

// TestNeedsDiscoveryWithoutConfiguredDevices keeps a misconfigured exporter quiet on the wire.
// With no devices configured nothing is ever exported, so discovering is pure noise.
func TestNeedsDiscoveryWithoutConfiguredDevices(t *testing.T) {
	l := NewListener(&config.Config{FetchInterval: time.Second}, collector.NewCollector())

	assert.False(t, l.needsDiscovery(time.Now()))
}

// TestDiscoverySettingsFallBackToDefaults keeps a programmatically constructed Listener safe
// even though production configs are defaulted at load time: a zero interval would make the
// loop rediscover without pause, and a zero window would make every cycle a no-op.
func TestDiscoverySettingsFallBackToDefaults(t *testing.T) {
	l := NewListener(&config.Config{}, collector.NewCollector())

	assert.Equal(t, config.DefaultDiscoveryInterval, l.discoveryInterval())
	assert.Equal(t, config.DefaultDiscoveryWindow, l.discoveryWindow())
	assert.Equal(t, config.DefaultFetchInterval, l.fetchInterval(),
		"a zero fetch interval would panic time.NewTicker")
}

func TestDiscoverySettingsAreConfigurable(t *testing.T) {
	l := testListener(t)
	l.cfg.Discovery.Interval = 30 * time.Minute
	l.cfg.Discovery.Window = 5 * time.Second

	assert.Equal(t, 30*time.Minute, l.discoveryInterval())
	assert.Equal(t, 5*time.Second, l.discoveryWindow())
}

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

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyDefaultsFillsAnEmptyConfig is the case that used to crash the exporter: a
// user-mounted config that omits fetchInterval unmarshalled to 0, and time.NewTicker panics
// on a non-positive duration.
func TestApplyDefaultsFillsAnEmptyConfig(t *testing.T) {
	c := Config{}

	c.ApplyDefaults()

	assert.Equal(t, DefaultFetchInterval, c.FetchInterval)
	assert.Equal(t, DefaultDiscoveryInterval, c.Discovery.Interval)
	assert.Equal(t, DefaultDiscoveryWindow, c.Discovery.Window)
	assert.Equal(t, DefaultEnergyMeterPrefix, c.Metrics.EnergyMeterPrefix)
	assert.Equal(t, DefaultInverterPrefix, c.Metrics.InverterPrefix)
	assert.Equal(t, DefaultPort, c.Exporter.Port)
	assert.NoError(t, c.Validate(), "a defaulted config must be usable")
}

func TestApplyDefaultsKeepsConfiguredValues(t *testing.T) {
	c := Config{
		FetchInterval: 10 * time.Second,
		Exporter:      ExporterConfig{Port: 9090},
		Discovery:     DiscoveryConfig{Interval: time.Hour, Window: time.Second},
		Metrics:       MetricsConfig{EnergyMeterPrefix: "em", InverterPrefix: "inv"},
	}

	c.ApplyDefaults()

	assert.Equal(t, 10*time.Second, c.FetchInterval)
	assert.Equal(t, uint16(9090), c.Exporter.Port)
	assert.Equal(t, time.Hour, c.Discovery.Interval)
	assert.Equal(t, time.Second, c.Discovery.Window)
	assert.Equal(t, "em", c.Metrics.EnergyMeterPrefix)
	assert.Equal(t, "inv", c.Metrics.InverterPrefix)
}

// TestValidateRejectsNonPositiveDurations covers values a user set on purpose and got wrong.
// Defaults cannot help here - 0 means "unset", but a negative duration is a mistake that
// deserves a clear error rather than a silent substitution.
func TestValidateRejectsNonPositiveDurations(t *testing.T) {
	tests := map[string]func(*Config){
		"fetchInterval":      func(c *Config) { c.FetchInterval = -time.Second },
		"discovery.interval": func(c *Config) { c.Discovery.Interval = -time.Second },
		"discovery.window":   func(c *Config) { c.Discovery.Window = -time.Second },
	}
	for key, break_ := range tests {
		t.Run(key, func(t *testing.T) {
			c := Config{}
			c.ApplyDefaults()
			break_(&c)

			err := c.Validate()

			require.Error(t, err)
			assert.Contains(t, err.Error(), key, "the error must name the offending key")
		})
	}
}

// TestValidateRejectsWindowNotShorterThanInterval guards the property that made discovery a
// problem for the rest of the network in the first place: a window at least as long as the
// interval means discovery runs continuously, broadcasting to the shared multicast group
// without pause.
func TestValidateRejectsWindowNotShorterThanInterval(t *testing.T) {
	c := Config{}
	c.ApplyDefaults()
	c.Discovery.Window = c.Discovery.Interval

	err := c.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "discovery.window")
}

func TestValidateRejectsEmptyMetricPrefixes(t *testing.T) {
	for _, key := range []string{"metrics.energyMeterPrefix", "metrics.inverterPrefix"} {
		t.Run(key, func(t *testing.T) {
			c := Config{}
			c.ApplyDefaults()
			if key == "metrics.energyMeterPrefix" {
				c.Metrics.EnergyMeterPrefix = ""
			} else {
				c.Metrics.InverterPrefix = ""
			}

			err := c.Validate()

			require.Error(t, err)
			assert.Contains(t, err.Error(), key)
		})
	}
}

// TestValidateReportsEveryProblemAtOnce keeps a misconfigured deployment from turning into a
// sequence of restart-and-find-the-next-error rounds.
func TestValidateReportsEveryProblemAtOnce(t *testing.T) {
	c := Config{FetchInterval: -time.Second, Discovery: DiscoveryConfig{Interval: -time.Second, Window: time.Second}}

	err := c.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetchInterval")
	assert.Contains(t, err.Error(), "discovery.interval")
}

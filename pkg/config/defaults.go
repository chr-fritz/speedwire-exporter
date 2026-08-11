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
	"errors"
	"fmt"
	"strings"
	"time"
)

// Defaults for every value that has one. They are the single source of truth: defaultConfig.yaml
// documents them, but it only ships at /etc/speedwire-exporter/config.yaml, which the container
// declares a VOLUME - so a user-mounted config replaces it wholesale and any key it omits
// arrives here as a zero value.
const (
	DefaultPort              uint16 = 8080
	DefaultFetchInterval            = 5 * time.Second
	DefaultDiscoveryInterval        = 5 * time.Minute
	DefaultDiscoveryWindow          = 3 * time.Second
	DefaultEnergyMeterPrefix        = "smartmeter"
	DefaultInverterPrefix           = "sunny_inverter"
)

// ApplyDefaults fills in every value the configuration left unset. Call it right after
// unmarshalling, before Validate.
//
// A zero value means "not configured" and is replaced silently; a value that is present but
// unusable is Validate's business, so that a typo produces an error instead of being quietly
// swapped for something the user did not ask for.
func (c *Config) ApplyDefaults() {
	if c.Exporter.Port == 0 {
		c.Exporter.Port = DefaultPort
	}
	if c.FetchInterval == 0 {
		c.FetchInterval = DefaultFetchInterval
	}
	if c.Discovery.Interval == 0 {
		c.Discovery.Interval = DefaultDiscoveryInterval
	}
	if c.Discovery.Window == 0 {
		c.Discovery.Window = DefaultDiscoveryWindow
	}
	if c.Metrics.EnergyMeterPrefix == "" {
		c.Metrics.EnergyMeterPrefix = DefaultEnergyMeterPrefix
	}
	if c.Metrics.InverterPrefix == "" {
		c.Metrics.InverterPrefix = DefaultInverterPrefix
	}
}

// Validate reports every reason the configuration cannot be used, joined into one error.
//
// Reporting all of them at once matters for a container that exits on a bad config: otherwise
// fixing a deployment becomes a sequence of restart-and-find-the-next-error rounds.
func (c Config) Validate() error {
	var errs []error

	if c.FetchInterval <= 0 {
		errs = append(errs, fmt.Errorf("fetchInterval must be positive, got %s", c.FetchInterval))
	}
	if c.Discovery.Interval <= 0 {
		errs = append(errs, fmt.Errorf("discovery.interval must be positive, got %s", c.Discovery.Interval))
	}
	if c.Discovery.Window <= 0 {
		errs = append(errs, fmt.Errorf("discovery.window must be positive, got %s", c.Discovery.Window))
	}
	// A window at least as long as the interval means discovery never pauses, which floods the
	// shared Speedwire multicast group with discovery requests for as long as the process runs.
	if c.Discovery.Window > 0 && c.Discovery.Interval > 0 && c.Discovery.Window >= c.Discovery.Interval {
		errs = append(errs, fmt.Errorf(
			"discovery.window (%s) must be shorter than discovery.interval (%s), otherwise discovery never pauses",
			c.Discovery.Window, c.Discovery.Interval))
	}
	if c.Metrics.EnergyMeterPrefix == "" {
		errs = append(errs, errors.New("metrics.energyMeterPrefix must not be empty"))
	}
	if c.Metrics.InverterPrefix == "" {
		errs = append(errs, errors.New("metrics.inverterPrefix must not be empty"))
	}
	for i, d := range c.Devices {
		// The port is appended when the device is opened, so anything with a colon - a port, or
		// an IPv6 literal - resolves as "host:9522:9522" and fails much later and much less
		// clearly. Rejecting every colon is safe: the Speedwire group is IPv4 only.
		if d.IsPinned() && strings.Contains(d.Address, ":") {
			errs = append(errs, fmt.Errorf(
				"devices[%d].address must not contain a port and must be IPv4 or a hostname, got %q",
				i, d.Address))
		}
	}

	return errors.Join(errs...)
}

package config

import "time"

type Config struct {
	Exporter      ExporterConfig
	Interface     string
	FetchInterval time.Duration
	Discovery     DiscoveryConfig
	Metrics       MetricsConfig
	Devices       []DeviceConfig
}

type ExporterConfig struct {
	Port      uint16
	GoMetrics bool
}

type DiscoveryConfig struct {
	Password string
	// Interval is how often a discovery cycle may run; Window bounds a single cycle. Zero
	// means "not configured" and falls back to the built-in default, so a partial user
	// config cannot silently reduce either to nothing.
	Interval time.Duration
	Window   time.Duration
}

type MetricsConfig struct {
	EnergyMeterPrefix string
	InverterPrefix    string
	Info              bool
}

type DeviceConfig struct {
	Serial uint32
	// Address pins the device to a known host, so it can be opened directly instead of being
	// looked for with a multicast discovery broadcast. Without a port - Speedwire is always
	// on 9522.
	Address string
	Labels  map[string]string
}

// IsPinned reports whether this device is reached by address instead of by discovery.
func (d DeviceConfig) IsPinned() bool { return d.Address != "" }

// LabelsFor returns the configured labels for the given serial and whether it is configured.
func (c Config) LabelsFor(serial uint32) (map[string]string, bool) {
	for _, d := range c.Devices {
		if d.Serial == serial {
			return d.Labels, true
		}
	}
	return nil, false
}

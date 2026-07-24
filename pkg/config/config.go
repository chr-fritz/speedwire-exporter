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
}

type MetricsConfig struct {
	EnergyMeterPrefix string
	InverterPrefix    string
	Info              bool
}

type DeviceConfig struct {
	Serial uint32
	Labels map[string]string
}

// LabelsFor returns the configured labels for the given serial and whether it is configured.
func (c Config) LabelsFor(serial uint32) (map[string]string, bool) {
	for _, d := range c.Devices {
		if d.Serial == serial {
			return d.Labels, true
		}
	}
	return nil, false
}

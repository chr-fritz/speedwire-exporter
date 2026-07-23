# Speedwire Prometheus Exporter

A Prometheus exporter for SMA devices communicating via the **Speedwire**
protocol. It reads values from SMA **Energy Meters** (Home Manager) and
**inverters** using the [`bboehmke/sunny`](https://gitlab.com/bboehmke/sunny)
library (the same library used by [evcc](https://evcc.io)) and exposes them as
Prometheus metrics.

## Planned features

- Reads SMA Energy Meters and inverters discovered via Speedwire multicast.
- Energy Meter: signed power gauges (feed-in negative) plus directional energy
  counters.
- Configurable metric prefixes per device type; which metrics are exported is
  driven by the devices registered in the configuration.
- A readout mode (CLI + `/devices` HTTP endpoint) to discover devices and their
  available values.
- Health checks and OpenTelemetry tracing.

## License

Apache License 2.0 — see [LICENSE](LICENSE).

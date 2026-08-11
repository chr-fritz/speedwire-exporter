# Speedwire Prometheus Exporter

[![Go build](https://github.com/chr-fritz/speedwire-exporter/actions/workflows/go.yaml/badge.svg)](https://github.com/chr-fritz/speedwire-exporter/actions/workflows/go.yaml)

A Prometheus exporter for SMA devices that communicate over the **Speedwire**
protocol. It reads SMA **Energy Meters** (Home Manager) and **inverters** using the [
`bboehmke/sunny`](https://gitlab.com/bboehmke/sunny) library — the same library used by [evcc](https://evcc.io) — and
exposes their values as Prometheus metrics.

## Features

- **Energy Meter** support: three-phase, bidirectional measurements exported as **signed** power gauges (feed-in is
  negative) plus per-direction energy counters — see [Metrics](#metrics).
- **Inverter** support: AC/DC power, voltage, current, energy, temperature, and hybrid-inverter values (PV / grid /
  consumption / battery).
- **Config-driven:** only the devices you register in the configuration are exported, with the labels you choose. Metric
  name prefixes are configurable per device type.
- **Discovery/readout** via a CLI command and an HTTP `/devices` endpoint to inspect every device on the wire and all
  the values it reports — handy for writing the configuration.
- Health checks (`/live`, `/ready`) and OpenTelemetry tracing.

## How it works

The exporter discovers Speedwire devices on the configured network interface via multicast, then periodically reads the
current values of every **configured**
device. A pure mapping layer turns each device's raw value set into named, labelled metrics (computing signed power,
converting energy units); a push-based collector caches the latest values and serves them on `/metrics`. Values expire
from the cache automatically if a device stops reporting.

## Installation

### Docker

```shell
docker pull ghcr.io/chr-fritz/speedwire-exporter
```

The image expects a configuration at `/etc/speedwire-exporter/config.yaml` and runs
`run --config /etc/speedwire-exporter/config.yaml` by default. The container needs access to the SMA Speedwire multicast
group (`239.12.255.254:9522`) on the relevant network — see
[Network requirements](#network-requirements).

### Binary / packages

Download the [latest release](https://github.com/chr-fritz/speedwire-exporter/releases/latest)
for your platform. Releases also include Debian/Ubuntu `.deb` packages (with a systemd unit).

## Configuration

Configuration is read from a YAML file (`--config`), from
`$HOME/.speedwire-exporter.yaml`, or from environment variables. A complete, annotated default lives in [
`defaultConfig.yaml`](defaultConfig.yaml):

```yaml
exporter:
    port: 8080          # port the /metrics, /devices, /live, /ready endpoints listen on
    goMetrics: false    # also export Go runtime + process metrics
logging:
    level: info         # error | warn | info | debug | debug-4
    format: json        # json | text
fetchInterval: 5s     # how often configured devices are read
interface: eth0       # network interface to discover/receive Speedwire on
discovery:
    password: "0000"    # inverter login password used during discovery
metrics:
    energyMeterPrefix: smartmeter       # metric name prefix for energy meters
    inverterPrefix: sunny_inverter      # metric name prefix for inverters
    info: false                         # also emit a <prefix>_info metric (software version, device name)
devices: # only listed serials are exported; labels are attached verbatim
    -   serial: 1234567890
        labels:
            meter: grid
```

| Key                                                    | Meaning                                                     |
|--------------------------------------------------------|-------------------------------------------------------------|
| `exporter.port` / `exporter.goMetrics`                 | Listen port; whether to include Go/process metrics.         |
| `logging.level` / `logging.format`                     | Log verbosity and format.                                   |
| `fetchInterval`                                        | Poll interval for reading each configured device.           |
| `interface`                                            | Interface used to discover and receive Speedwire multicast. |
| `discovery.password`                                   | Password used to log in to inverters during discovery.      |
| `metrics.energyMeterPrefix` / `metrics.inverterPrefix` | Metric name prefixes per device type.                       |
| `metrics.info`                                         | Emit the optional `<prefix>_info` metric.                   |
| `devices[].serial` / `devices[].labels`                | Which devices to export and the constant labels to attach.  |

> **Note:** all devices of the *same type* should declare the *same set of label
> keys*. Differing label-key sets across same-type devices produce metric series
> with inconsistent dimensions, which Prometheus rejects on scrape.

### Environment variables

Any config key can be overridden via an environment variable prefixed with
`SPEEDWIRE_`, with dots replaced by underscores (e.g. `discovery.password` →
`SPEEDWIRE_DISCOVERY_PASSWORD`). This is the recommended way to supply secrets like the discovery password without
writing them to disk:

```shell
SPEEDWIRE_DISCOVERY_PASSWORD=s3cret speedwire-exporter run --config /etc/speedwire-exporter/config.yaml
```

Env vars take precedence over the config file but not over explicit CLI flags.

## Usage

```shell
speedwire-exporter run --config /etc/speedwire-exporter/config.yaml
```

`run` flags:

| Flag                  | Default | Description                                 |
|-----------------------|---------|---------------------------------------------|
| `-p, --port`          | `8080`  | Port to expose metrics on.                  |
| `-g, --withGoMetrics` | `false` | Also export Go runtime and process metrics. |
| `--config`            | —       | Config file path.                           |
| `-v, --log_level`     | `info`  | Minimum log level.                          |
| `--log_format`        | `text`  | Log format (`text` or `json`).              |

### HTTP endpoints

| Path              | Purpose                                                                                          |
|-------------------|--------------------------------------------------------------------------------------------------|
| `/metrics`        | Prometheus metrics (OpenMetrics enabled).                                                        |
| `/devices`        | JSON dump of all discovered devices and their current values — independent of the configuration. |
| `/live`, `/ready` | Liveness / readiness health checks.                                                              |

### Discovering devices

To find device serial numbers and the values they report (e.g. to build the
`devices:` config), either query `/devices` on a running exporter, or run the one-shot CLI:

```shell
speedwire-exporter readout --config config.yaml
```

Shell completion scripts are available via `speedwire-exporter completion <bash|zsh|fish|ps1>`.

## Metrics

Constant labels come from each device's `labels:` in the configuration (e.g.
`meter="grid"`); the labels below are added by the exporter. Names use the configured prefixes (defaults `smartmeter` /
`sunny_inverter`).

### Energy Meter (`smartmeter_*`)

Power is **signed** (feed-in negative); energy counters are split by direction.

| Metric                             | Type    | Unit  | Labels                                              |
|------------------------------------|---------|-------|-----------------------------------------------------|
| `smartmeter_active_power`          | gauge   | W     | `phase`                                             |
| `smartmeter_reactive_power`        | gauge   | var   | `phase`                                             |
| `smartmeter_apparent_power`        | gauge   | VA    | `phase`                                             |
| `smartmeter_current`               | gauge   | A     | `phase`                                             |
| `smartmeter_voltage`               | gauge   | V     | `phase`                                             |
| `smartmeter_power_factor`          | gauge   | —     | `phase`                                             |
| `smartmeter_frequency`             | gauge   | Hz    | —                                                   |
| `smartmeter_energy_total`          | counter | kWh   | `phase`, `direction`                                |
| `smartmeter_reactive_energy_total` | counter | kvarh | `phase`, `direction`                                |
| `smartmeter_apparent_energy_total` | counter | kVAh  | `phase`, `direction`                                |
| `smartmeter_info`                  | gauge   | —     | `software_version` (only when `metrics.info: true`) |

`phase` ∈ `total`, `l1`, `l2`, `l3`; `direction` ∈ `consumption`, `delivery`.

### Inverter (`sunny_inverter_*`)

| Metric                                   | Type    | Unit | Labels                                                             |
|------------------------------------------|---------|------|--------------------------------------------------------------------|
| `sunny_inverter_power`                   | gauge   | W    | `side`, `phase`                                                    |
| `sunny_inverter_power_max`               | gauge   | W    | —                                                                  |
| `sunny_inverter_voltage`                 | gauge   | V    | `side`, `phase`                                                    |
| `sunny_inverter_current`                 | gauge   | A    | `side`, `phase`                                                    |
| `sunny_inverter_frequency`               | gauge   | Hz   | —                                                                  |
| `sunny_inverter_temperature`             | gauge   | °C   | —                                                                  |
| `sunny_inverter_energy`                  | counter | kWh  | `interval`, `direction`                                            |
| `sunny_inverter_energy_today`            | gauge   | kWh  | —                                                                  |
| `sunny_inverter_pv_power`                | gauge   | W    | —                                                                  |
| `sunny_inverter_pv_energy`               | counter | kWh  | —                                                                  |
| `sunny_inverter_grid_power`              | gauge   | W    | `direction`                                                        |
| `sunny_inverter_grid_energy`             | counter | kWh  | `direction`, `interval`                                            |
| `sunny_inverter_grid_energy_today`       | gauge   | kWh  | `direction`                                                        |
| `sunny_inverter_consumption_power`       | gauge   | W    | —                                                                  |
| `sunny_inverter_consumption_energy`      | counter | kWh  | —                                                                  |
| `sunny_inverter_self_consumption_power`  | gauge   | W    | —                                                                  |
| `sunny_inverter_self_consumption_energy` | counter | kWh  | —                                                                  |
| `sunny_inverter_battery_charge`          | gauge   | %    | —                                                                  |
| `sunny_inverter_battery_energy`          | counter | kWh  | `direction`                                                        |
| `sunny_inverter_battery_voltage`         | gauge   | V    | —                                                                  |
| `sunny_inverter_battery_current`         | gauge   | A    | —                                                                  |
| `sunny_inverter_battery_temperature`     | gauge   | °C   | —                                                                  |
| `sunny_inverter_battery_charge_cycles`   | gauge   | —    | —                                                                  |
| `sunny_inverter_info`                    | gauge   | —    | `software_version`, `device_name` (only when `metrics.info: true`) |

`side` ∈ `AC`, `DC`. Values a given device does not report are simply omitted. Hybrid-inverter metrics (`pv_*`,
`grid_*`, `consumption_*`,
`self_consumption_*`, `battery_*`) require an inverter that provides them (e.g. SMA STP … SE hybrid inverters).

### Exporter health (`speedwire_*`)

| Metric                                             | Type  | Unit | Labels   |
|----------------------------------------------------|-------|------|----------|
| `speedwire_last_successful_read_timestamp_seconds` | gauge | s    | `serial` |

Device values expire from the cache 30 s after readings stop, so an exporter that has lost the
multicast stream serves an empty `/metrics` — indistinguishable from one that is simply idle.
This gauge keeps reporting, one series per configured device, and makes that alertable. A
device that has never been read reports `0`, so a device that never came up at all stays
visible rather than silently absent — which is why the `== 0` arm below is needed:

```promql
speedwire_last_successful_read_timestamp_seconds == 0
  or time() - speedwire_last_successful_read_timestamp_seconds > 300
```

Give the rule a `for:` long enough to cover startup, since every device reports `0` until its
first successful read.

## Observability

OpenTelemetry tracing is set up via the OTel SDK autoexport; configure it with the standard `OTEL_*` environment
variables (e.g.
`OTEL_EXPORTER_OTLP_ENDPOINT`). Request tracing is applied to the HTTP handlers.

The Speedwire layer's own trace output is logged at `logging.level: debug`: every datagram
sent and received, malformed packets, and which devices discovery found or skipped. That is
one line per datagram on a live segment, so use it to answer "does anything reach us at all?"
rather than leaving it on. `debug-4` additionally logs the packets that were *dropped* (failed
UDP reads, receivers whose channel was full), which is what to reach for when values arrive
only intermittently.

## Network requirements

The exporter must have layer-2 access to the SMA Speedwire multicast group
`239.12.255.254:9522` on the configured `interface`. In Kubernetes this typically means attaching the pod to the VLAN
carrying the SMA traffic (e.g. via Multus/macvlan) so the multicast datagrams are received.

## Building from source

```shell
make build          # build ./bin/speedwire-exporter
make test           # run tests with coverage
make ci-check       # tidy, generate, imports, vet, test (what CI runs)
```

Requires Go (see the `go` directive in [`go.mod`](go.mod) for the minimum version; CI always builds with the latest
stable Go).

## Contributing

1. Fork the repository.
2. Create a feature branch (`git checkout -b my-feature`).
3. Make your changes with tests and run `make ci-check`.
4. Push and open a pull request.

## License

Apache License 2.0 — see [LICENSE](LICENSE).

## Maintainer

Christian Fritz ([@chr-fritz](https://github.com/chr-fritz))

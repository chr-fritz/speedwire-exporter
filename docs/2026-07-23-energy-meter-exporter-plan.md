# Speedwire Exporter Implementation Plan (P0 + P1 + P2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:
> executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A native Go Prometheus exporter that reads SMA devices via Speedwire multicast (using `bboehmke/sunny`):
energy meters → `smartmeter_*` (signed power gauges, directional energy counters) and inverters → `sunny_inverter_*`
(incl. sunny 0.17.0 hybrid values).

**Architecture:** A Speedwire listener discovers devices and periodically reads their values; a pure mapping function
turns the raw `sunny` value map into bare metric snapshots (signing + unit conversion); a push-based custom
`prometheus.Collector` (expiring cache) exposes them. HTTP server, logging (slog + trace correlation) and OpenTelemetry
are ported from the sibling `d0-smartmeter` exporter.

**Tech Stack:** Go (latest toolchain), `gitlab.com/bboehmke/sunny`, `prometheus/client_golang`, `patrickmn/go-cache`,
`spf13/cobra`+`viper`, OpenTelemetry (`autoexport`, `otelhttp`), `heptiolabs/healthcheck`.

## Global Constraints

- Module path: `github.com/chr-fritz/speedwire-exporter`.
- Use the **latest version of every dependency** — after adding deps run `go get -u -t ./... && go mod tidy`.
- License headers: keep the Apache-2.0 file header comment used across the codebase (copy from any ported
  `d0-smartmeter` file).
- Metric prefixes are **configurable per device type** (`metrics.energyMeterPrefix` default `smartmeter`,
  `metrics.inverterPrefix` default `sunny_inverter`). Mapping functions emit **bare** names (no prefix); the collector
  prepends the configured prefix.
- Energy Meter energy counters are in **kWh** (`Ws ÷ 3 600 000`).
- Power gauges are **signed**: feed-in (Minus/`supply`) negative, `value = plus − minus`.
- `direction` label values are `consumption` (Plus) and `delivery` (Minus).
- `phase` label values are `total`, `l1`, `l2`, `l3`.
- Unmapped `sunny` values are logged **once per ValueID** at `info` level, never repeatedly.
- Reference sources (read-only, same machine): `/Users/christian.fritz/Development/chr-fritz/d0-smartmeter`
  (architecture) and the `main-old` branch of this repo (original scaffold + Speedwire PoC). `sunny` value catalog:
  module cache `gitlab.com/bboehmke/sunny@latest/values.go`.

---

## File Structure

**P0 (scaffold):**

- `go.mod`, `go.sum` — module + deps
- `main.go` — calls `cmd.Execute()`
- `version/version.go` — build metadata (AppName/Version/Revision/Branch/CommitDate)
- `cmd/root.go` — cobra root + viper config init
- `cmd/logging.go` — logging flags wiring
- `cmd/version.go` — `version` subcommand
- `cmd/completion.go` — shell completion subcommand
- `cmd/run.go` — `run` subcommand: wire server + collector + listener
- `cmd/readout.go` — `readout` subcommand: discover + dump values
- `cmd/devices.go` — `/devices` HTTP handler: JSON dump of discovered devices
- `pkg/logging/logging.go`, `pkg/logging/tracing-log-handler.go` — slog + trace correlation
- `pkg/observerbility/otel.go` — OpenTelemetry SDK bootstrap
- `pkg/server/server.go` — HTTP server (otelhttp, health, graceful shutdown)
- `pkg/config/config.go` — config structs
- `defaultConfig.yaml` — sample config
- `Makefile`, `Dockerfile`, `.goreleaser.yaml`, `.gitignore`, `LICENSE`, `sonar-project.properties`
- `.github/workflows/go.yaml`, `.github/workflows/release.yaml`, `.github/dependabot.yml`

**P1 (Energy Meter):**

- `pkg/mapping/types.go` — `Snapshot`, `MetricType`, `toFloat`
- `pkg/mapping/energymeter.go` — `MapEnergyMeter`
- `pkg/mapping/energymeter_test.go`
- `pkg/collector/collector.go` — push collector
- `pkg/collector/collector_test.go`
- `pkg/speedwire/listener.go` — discovery + read loop → collector
- Wiring in `cmd/run.go`, `cmd/readout.go`

**P2 (inverters):**

- `pkg/mapping/inverter.go` — `MapInverter`, `IsMappedInverter`, `InverterInfo`
- `pkg/mapping/inverter_test.go`
- Inverter branch in `pkg/speedwire/listener.go`

---

## P0 — Scaffold

### Task 1: Go module, version package, main, root command

**Files:**

- Create: `go.mod`, `main.go`, `version/version.go`, `cmd/root.go`, `cmd/version.go`, `cmd/completion.go`

**Interfaces:**

- Produces: `version.AppName` (string `"speedwire-exporter"`), `version.Version/Revision/Branch/CommitDate` (strings,
  set via ldflags); `cmd.Execute()`; package-level `rootCmd *cobra.Command`.

- [ ] **Step 1: Initialise the module and add core deps**

```bash
cd /Users/christian.fritz/Development/chr-fritz/speedwire-exporter
go mod init github.com/chr-fritz/speedwire-exporter
go get github.com/spf13/cobra@latest github.com/spf13/pflag@latest github.com/spf13/viper@latest \
       github.com/mitchellh/go-homedir@latest github.com/stretchr/testify@latest
```

- [ ] **Step 2: Create `version/version.go`** (copy the file header from `d0-smartmeter`)

```go
package version

// Set via -ldflags at build time.
var (
    AppName    = "speedwire-exporter"
    Version    = "dev"
    Revision   = "unknown"
    Branch     = "unknown"
    CommitDate = "unknown"
)
```

- [ ] **Step 3: Create `cmd/root.go`** — port from `d0-smartmeter/cmd/root.go`, replacing the command `Use` and default
  config name.

Copy `/Users/christian.fritz/Development/chr-fritz/d0-smartmeter/cmd/root.go` verbatim, then change:
`Use: "speedwire-exporter"`, `Short: "Exports SMA Speedwire values to Prometheus"`, and the config name
`SetConfigName(".speedwire-exporter")`.

- [ ] **Step 4: Create `cmd/version.go` and `cmd/completion.go`** — port from the corresponding
  `d0-smartmeter/cmd/version.go` and `cmd/completion.go` verbatim (they only depend on `version.*` and cobra).

- [ ] **Step 5: Create `main.go`**

```go
package main

import "github.com/chr-fritz/speedwire-exporter/cmd"

func main() {
    cmd.Execute()
}
```

- [ ] **Step 6: Build and smoke-test**

Run: `go get -u -t ./... && go mod tidy && go build ./... && go run . version`
Expected: build succeeds; `version` prints `speedwire-exporter dev …`.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum main.go version cmd
git commit -m "Add module skeleton with root, version and completion commands"
```

---

### Task 2: Logging package

**Files:**

- Create: `pkg/logging/logging.go`, `pkg/logging/tracing-log-handler.go`, `cmd/logging.go`
- Test: `pkg/logging/logging_test.go`

**Interfaces:**

- Produces: `logging.InitFlags(flagset *pflag.FlagSet, cmd *cobra.Command) LoggerConfiguration` with `Initialize()`;
  `logging.NewTracingLogHandler(parent slog.Handler) slog.Handler`.

- [ ] **Step 1: Port the logging files**

Copy verbatim from `d0-smartmeter`:

- `pkg/logging/logging.go`
- `pkg/logging/tracing-log-handler.go`
- `pkg/logging/logging_test.go`

No code changes needed (package is self-contained). Add the OTel/glog deps they import:

```bash
go get go.opentelemetry.io/otel/trace@latest
```

- [ ] **Step 2: Create `cmd/logging.go`** — port from `d0-smartmeter/cmd/logging.go`, changing the import path to
  `github.com/chr-fritz/speedwire-exporter/pkg/logging`. (Drop the `glog` line if `glog` is unused after porting; keep
  only if `logging.go` requires it.)

- [ ] **Step 3: Run the ported tests**

Run: `go test ./pkg/logging/... -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/logging cmd/logging.go go.mod go.sum
git commit -m "Port slog logging with trace-correlation handler"
```

---

### Task 3: OpenTelemetry package

**Files:**

- Create: `pkg/observerbility/otel.go`

**Interfaces:**

- Produces: `observerbility.SetupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error)`;
  `observerbility.GetTracer() trace.Tracer`.

- [ ] **Step 1: Port `pkg/observerbility/otel.go`** from `d0-smartmeter`, changing the version import to
  `github.com/chr-fritz/speedwire-exporter/version` and the tracer name in `GetTracer()` to `"speedwire-exporter"`. Add
  deps:

```bash
go get go.opentelemetry.io/contrib/exporters/autoexport@latest \
       go.opentelemetry.io/otel@latest go.opentelemetry.io/otel/sdk@latest
```

- [ ] **Step 2: Build**

Run: `go build ./pkg/observerbility/...`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add pkg/observerbility go.mod go.sum
git commit -m "Port OpenTelemetry SDK bootstrap"
```

---

### Task 4: HTTP server package

**Files:**

- Create: `pkg/server/server.go`

**Interfaces:**

- Produces: `server.NewHttpServer(port uint16) (*HttpServer, error)`; methods
  `AddHandler(pattern string, handler http.Handler)`, `AddHandleFunc`,
  `AddLivenessCheck(name string, check healthcheck.Check)`, `AddReadinessCheck`, `Addr() net.Addr`,
  `Run(ctx context.Context) error`.

- [ ] **Step 1: Port `pkg/server/server.go`** from `d0-smartmeter` verbatim, changing the import to
  `github.com/chr-fritz/speedwire-exporter/pkg/observerbility`. Add deps:

```bash
go get github.com/heptiolabs/healthcheck@latest \
       go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@latest
```

- [ ] **Step 2: Write a smoke test** `pkg/server/server_test.go`

```go
package server

import (
    "context"
    "net/http"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestServerServesLiveEndpoint(t *testing.T) {
    s, err := NewHttpServer(0) // :0 => random free port
    require.NoError(t, err)

    ctx, cancel := context.WithCancel(context.Background())
    go func() { _ = s.Run(ctx) }()
    defer cancel()

    time.Sleep(100 * time.Millisecond)
    resp, err := http.Get("http://" + s.Addr().String() + "/live")
    require.NoError(t, err)
    defer resp.Body.Close()
    assert.Equal(t, http.StatusOK, resp.StatusCode)
}
```

- [ ] **Step 3: Run the test**

Run: `go test ./pkg/server/... -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/server go.mod go.sum
git commit -m "Port HTTP server with otelhttp, health checks and graceful shutdown"
```

---

### Task 5: Config package

**Files:**

- Create: `pkg/config/config.go`, `defaultConfig.yaml`
- Test: `pkg/config/config_test.go`

**Interfaces:**

- Produces:

```go
type Config struct {
Exporter      ExporterConfig
Interface     string
FetchInterval time.Duration
Discovery     DiscoveryConfig
Metrics       MetricsConfig
Devices       []DeviceConfig
}
type ExporterConfig struct { Port uint16; GoMetrics bool }
type DiscoveryConfig struct { Password string }
type MetricsConfig struct { EnergyMeterPrefix string; InverterPrefix string; Info bool }
type DeviceConfig struct { Serial uint32; Labels map[string]string }
// LabelsFor(serial) (map[string]string, bool) returns the configured labels for a serial.
```

- [ ] **Step 1: Write the failing test** `pkg/config/config_test.go`

```go
package config

import (
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestLabelsForReturnsConfiguredLabels(t *testing.T) {
    c := Config{Devices: []DeviceConfig{
        {Serial: 1234567890, Labels: map[string]string{"meter": "grid"}},
    }}
    labels, ok := c.LabelsFor(1234567890)
    assert.True(t, ok)
    assert.Equal(t, "grid", labels["meter"])

    _, ok = c.LabelsFor(999)
    assert.False(t, ok)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/config/... -run TestLabelsFor -v`
Expected: FAIL (undefined `Config`/`LabelsFor`).

- [ ] **Step 3: Implement `pkg/config/config.go`**

```go
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
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/config/... -run TestLabelsFor -v`
Expected: PASS.

- [ ] **Step 5: Create `defaultConfig.yaml`**

```yaml
exporter:
    port: 8080
    goMetrics: false
logging:
    level: info
    format: json
fetchInterval: 5s
interface: eth0
discovery:
    password: "0000"
metrics:
    energyMeterPrefix: smartmeter
    inverterPrefix: sunny_inverter
    info: false
devices: [ ]
#  - serial: 1234567890
#    labels:
#      meter: grid
```

- [ ] **Step 6: Commit**

```bash
git add pkg/config defaultConfig.yaml
git commit -m "Add config structs and default configuration"
```

---

### Task 6: Build, CI and packaging files

**Files:**

- Create: `Makefile`, `Dockerfile`, `.goreleaser.yaml`, `.gitignore`, `LICENSE`, `sonar-project.properties`,
  `.github/workflows/go.yaml`, `.github/workflows/release.yaml`, `.github/dependabot.yml`

- [ ] **Step 1: Port build/packaging files** from `d0-smartmeter`, replacing every occurrence of `d0-smartmeter` with
  `speedwire-exporter` and `ROOT_PACKAGE`/module paths accordingly:
- `Makefile` (update `NAME`, `ROOT_PACKAGE`)
- `Dockerfile` (update binary name, config path `/etc/speedwire-exporter/config.yaml`, user/group names; keep
  `busybox:stable-glibc`)
- `.goreleaser.yaml`, `.gitignore`, `sonar-project.properties`
- `.github/workflows/go.yaml`, `.github/workflows/release.yaml`, `.github/dependabot.yml`

- [ ] **Step 2: Create `LICENSE`** — copy the Apache-2.0 `LICENSE` file from `d0-smartmeter` verbatim.

- [ ] **Step 3: Verify the Makefile build target works**

Run: `make build`
Expected: produces `./bin/speedwire-exporter`; `./bin/speedwire-exporter version` runs.

- [ ] **Step 4: Commit**

```bash
git add Makefile Dockerfile .goreleaser.yaml .gitignore LICENSE sonar-project.properties .github
git commit -m "Add build, packaging and CI configuration"
```

---

### Task 7: `run` command wiring (server + empty registry)

**Files:**

- Create: `cmd/run.go`

**Interfaces:**

- Consumes: `server.NewHttpServer`, `config.Config`, viper config.
- Produces: `run` subcommand serving `/metrics` (Go metrics optional), `/live`, `/ready`.

- [ ] **Step 1: Implement `cmd/run.go`** (collector/listener wired in P1; here just server + registry)

```go
package cmd

import (
    "context"
    "log/slog"

    "github.com/chr-fritz/speedwire-exporter/pkg/config"
    "github.com/chr-fritz/speedwire-exporter/pkg/server"
    "github.com/heptiolabs/healthcheck"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

const RunPortParm = "exporter.port"
const WithGoMetricsParm = "exporter.goMetrics"

func NewRunCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "run",
        Short: "Run the exporter",
        Args:  cobra.NoArgs,
        Run:   run,
    }
    cmd.Flags().Uint16P("port", "p", 8080, "The port where metrics are exported.")
    cmd.Flags().BoolP("withGoMetrics", "g", false, "Also export Go runtime metrics.")
    _ = viper.BindPFlag(RunPortParm, cmd.Flags().Lookup("port"))
    _ = viper.BindPFlag(WithGoMetricsParm, cmd.Flags().Lookup("withGoMetrics"))
    return cmd
}

func run(cmd *cobra.Command, _ []string) {
    var cfg config.Config
    if err := viper.Unmarshal(&cfg); err != nil {
        slog.With("err", err).Error("Can not read configuration")
        return
    }

    ctx, cancel := context.WithCancel(cmd.Context())
    defer cancel()

    httpServer, err := server.NewHttpServer(uint16(viper.GetUint(RunPortParm)))
    if err != nil {
        slog.With("err", err).Error("Can not start http server")
        return
    }

    registry := newRegistry(viper.GetBool(WithGoMetricsParm))
    httpServer.AddHandler("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{EnableOpenMetrics: true}))
    httpServer.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(100))

    // P1: collector + speedwire listener are registered/started here.

    if err = httpServer.Run(ctx); err != nil {
        slog.With("err", err).Error("metrics server stopped")
    }
}

func newRegistry(withGo bool) *prometheus.Registry {
    if withGo {
        return prometheus.DefaultRegisterer.(*prometheus.Registry)
    }
    return prometheus.NewPedanticRegistry()
}

func init() {
    rootCmd.AddCommand(NewRunCommand())
}
```

Add dep: `go get github.com/prometheus/client_golang@latest`.

- [ ] **Step 2: Manual smoke test**

Run: `go run . run --port 18080` then in another shell `curl -s localhost:18080/metrics | head` and
`curl -s -o /dev/null -w "%{http_code}\n" localhost:18080/live`. Expected: `/metrics` returns (empty of custom metrics),
`/live` returns `200`.

- [ ] **Step 3: Commit**

```bash
git add cmd/run.go go.mod go.sum
git commit -m "Wire run command with HTTP server and metrics registry"
```

---

### Task 8: `readout` command, discovery helper and `/devices` endpoint

**Files:**

- Create: `cmd/readout.go`, `pkg/speedwire/discover.go`, `cmd/devices.go`
- Modify: `cmd/run.go`
- Test: `cmd/devices_test.go`

**Interfaces:**

- Produces:

```go
// pkg/speedwire
type DiscoveredDevice struct {
Serial        uint32
Address       string
IsEnergyMeter bool
Values        map[sunny.ValueID]interface{}
}
func Discover(ctx context.Context, iface, password string) ([]DiscoveredDevice, error)

// cmd
type DiscoverFunc func (ctx context.Context) ([]speedwire.DiscoveredDevice, error)
func newDevicesHandler(discover DiscoverFunc) http.HandlerFunc
```

- Consumes: `sunny.NewConnection`, `Connection.DiscoverDevices`, `Device.GetValuesCtx`,
  `Device.SerialNumber/Address/IsEnergyMeter`, `sunny.GetValueInfo`.

- [ ] **Step 1: Add the sunny dep**

```bash
go get gitlab.com/bboehmke/sunny@latest
```

- [ ] **Step 2: Implement `pkg/speedwire/discover.go`** (verify the `DiscoverDevices` signature against the installed
  version — the reference PoC is on branch `main-old`, `pkg/speedwire/fetcher.go`)

```go
package speedwire

import (
    "context"
    "time"

    "gitlab.com/bboehmke/sunny"
)

type DiscoveredDevice struct {
    Serial        uint32
    Address       string
    IsEnergyMeter bool
    Values        map[sunny.ValueID]interface{}
}

// Discover finds all Speedwire devices on the given interface and reads their current values once.
func Discover(ctx context.Context, iface, password string) ([]DiscoveredDevice, error) {
    conn, err := sunny.NewConnection(iface)
    if err != nil {
        return nil, err
    }

    devs := make(chan *sunny.Device)
    go conn.DiscoverDevices(ctx, devs, password)

    var result []DiscoveredDevice
    timeout := time.After(3 * time.Second)
    for {
        select {
        case dev := <-devs:
            if dev == nil {
                continue
            }
            values, _ := dev.GetValuesCtx(ctx)
            result = append(result, DiscoveredDevice{
                Serial:        dev.SerialNumber(),
                Address:       dev.Address().String(),
                IsEnergyMeter: dev.IsEnergyMeter(),
                Values:        values,
            })
        case <-timeout:
            return result, nil
        case <-ctx.Done():
            return result, ctx.Err()
        }
    }
}
```

Note: adjust `dev.Address()` to `.String()` only if `Address()` returns a `net.Addr`; if it already returns a string,
drop `.String()`. Verify against the installed `sunny` API.

- [ ] **Step 3: Implement `cmd/readout.go`** (CLI dump)

```go
package cmd

import (
    "context"
    "fmt"
    "time"

    "github.com/chr-fritz/speedwire-exporter/pkg/config"
    "github.com/chr-fritz/speedwire-exporter/pkg/speedwire"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
    "gitlab.com/bboehmke/sunny"
)

func NewReadoutCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "readout",
        Short: "Discover Speedwire devices and print all their current values",
        Args:  cobra.NoArgs,
        Run:   readout,
    }
}

func readout(cmd *cobra.Command, _ []string) {
    var cfg config.Config
    _ = viper.Unmarshal(&cfg)

    ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
    defer cancel()

    devices, err := speedwire.Discover(ctx, cfg.Interface, cfg.Discovery.Password)
    if err != nil {
        fmt.Println("discovery error:", err)
        return
    }
    for _, d := range devices {
        fmt.Printf("device %d @ %s (energyMeter=%t)\n", d.Serial, d.Address, d.IsEnergyMeter)
        for id, v := range d.Values {
            info := sunny.GetValueInfo(id)
            fmt.Printf("  %-40s %v %s\n", info.Description, v, info.Unit)
        }
    }
}

func init() {
    rootCmd.AddCommand(NewReadoutCommand())
}
```

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: succeeds. (Runtime discovery needs real hardware / correct interface; not asserted here.)

- [ ] **Step 5: Commit**

```bash
git add cmd/readout.go pkg/speedwire/discover.go go.mod go.sum
git commit -m "Add readout command and Speedwire discovery helper"
```

- [ ] **Step 6: Implement `cmd/devices.go`** — an HTTP handler that dumps all discovered devices (config-independent) as
  JSON, using an injectable discover func for testability.

```go
package cmd

import (
    "context"
    "encoding/json"
    "net/http"
    "time"

    "github.com/chr-fritz/speedwire-exporter/pkg/speedwire"
    "gitlab.com/bboehmke/sunny"
)

type DiscoverFunc func(ctx context.Context) ([]speedwire.DiscoveredDevice, error)

type deviceValueView struct {
    ID          int         `json:"id"`
    Description string      `json:"description"`
    Unit        string      `json:"unit"`
    Value       interface{} `json:"value"`
}

type deviceView struct {
    Serial        uint32            `json:"serial"`
    Address       string            `json:"address"`
    IsEnergyMeter bool              `json:"isEnergyMeter"`
    Values        []deviceValueView `json:"values"`
}

func newDevicesHandler(discover DiscoverFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
        defer cancel()

        devices, err := discover(ctx)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        views := make([]deviceView, 0, len(devices))
        for _, d := range devices {
            values := make([]deviceValueView, 0, len(d.Values))
            for id, v := range d.Values {
                info := sunny.GetValueInfo(id)
                values = append(values, deviceValueView{
                    ID: int(id), Description: info.Description, Unit: info.Unit, Value: v,
                })
            }
            views = append(views, deviceView{
                Serial: d.Serial, Address: d.Address, IsEnergyMeter: d.IsEnergyMeter, Values: values,
            })
        }

        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(views)
    }
}
```

- [ ] **Step 7: Wire `/devices` into `cmd/run.go`** — add the `speedwire` import and, after the `/metrics` handler
  registration, insert:

```go
    httpServer.AddHandleFunc("/devices", newDevicesHandler(func (ctx context.Context) ([]speedwire.DiscoveredDevice, error) {
return speedwire.Discover(ctx, cfg.Interface, cfg.Discovery.Password)
}))
```

Note: this opens a short-lived Speedwire connection per request. Once the P1 listener owns a persistent connection, both
rely on the multicast socket's `SO_REUSEADDR`; if a platform rejects the second bind, refactor `/devices` to read from
the listener's discovered-device set.

- [ ] **Step 8: Write and run the handler test** `cmd/devices_test.go`

```go
package cmd

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/chr-fritz/speedwire-exporter/pkg/speedwire"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "gitlab.com/bboehmke/sunny"
)

func TestDevicesHandlerReturnsJSON(t *testing.T) {
    h := newDevicesHandler(func(ctx context.Context) ([]speedwire.DiscoveredDevice, error) {
        return []speedwire.DiscoveredDevice{{
            Serial: 42, Address: "1.2.3.4:9522", IsEnergyMeter: true,
            Values: map[sunny.ValueID]interface{}{sunny.ActivePowerPlus: 15.4},
        }}, nil
    })

    rec := httptest.NewRecorder()
    h(rec, httptest.NewRequest(http.MethodGet, "/devices", nil))

    require.Equal(t, http.StatusOK, rec.Code)
    var out []map[string]interface{}
    require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
    require.Len(t, out, 1)
    assert.EqualValues(t, 42, out[0]["serial"])
    assert.Equal(t, true, out[0]["isEnergyMeter"])
}
```

Run: `go test ./cmd/... -run TestDevicesHandler -v`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add cmd/devices.go cmd/devices_test.go cmd/run.go
git commit -m "Add /devices HTTP readout endpoint"
```

---

## P1 — Energy Meter

### Task 9: Mapping types and `toFloat`

**Files:**

- Create: `pkg/mapping/types.go`
- Test: `pkg/mapping/types_test.go`

**Interfaces:**

- Produces:

```go
type MetricType int
const ( Gauge MetricType = iota; Counter )
type Snapshot struct { Name string; Type MetricType; Labels map[string]string; Value float64 }
func toFloat(v interface{}) (float64, bool) // unexported; used within package
```

- [ ] **Step 1: Write the failing test** `pkg/mapping/types_test.go`

```go
package mapping

import (
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestToFloatHandlesSunnyTypes(t *testing.T) {
    cases := []struct {
        in   interface{}
        want float64
        ok   bool
    }{
        {float64(1.5), 1.5, true},
        {uint64(42), 42, true},
        {uint32(7), 7, true},
        {int64(-3), -3, true},
        {nil, 0, false},
        {"nope", 0, false},
    }
    for _, c := range cases {
        got, ok := toFloat(c.in)
        assert.Equal(t, c.ok, ok)
        if c.ok {
            assert.Equal(t, c.want, got)
        }
    }
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/mapping/... -run TestToFloat -v`
Expected: FAIL (undefined `toFloat`).

- [ ] **Step 3: Implement `pkg/mapping/types.go`**

```go
package mapping

type MetricType int

const (
    Gauge MetricType = iota
    Counter
)

// Snapshot is a single mapped metric value with a bare (prefix-less) name.
type Snapshot struct {
    Name   string
    Type   MetricType
    Labels map[string]string
    Value  float64
}

func toFloat(v interface{}) (float64, bool) {
    switch n := v.(type) {
    case float64:
        return n, true
    case float32:
        return float64(n), true
    case uint64:
        return float64(n), true
    case uint32:
        return float64(n), true
    case int64:
        return float64(n), true
    case int32:
        return float64(n), true
    case int:
        return float64(n), true
    default:
        return 0, false
    }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/mapping/... -run TestToFloat -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/mapping/types.go pkg/mapping/types_test.go
git commit -m "Add mapping snapshot type and numeric coercion helper"
```

---

### Task 10: Signed power mapping (active/reactive/apparent)

**Files:**

- Create: `pkg/mapping/energymeter.go`
- Test: `pkg/mapping/energymeter_test.go`

**Interfaces:**

- Produces: `func MapEnergyMeter(values map[sunny.ValueID]interface{}) []Snapshot` (this task returns only the signed
  power snapshots; extended in Tasks 11–12).
- A signed-power snapshot has `Name` ∈ {`active_power`,`reactive_power`,`apparent_power`}, `Type=Gauge`,
  `Labels{"phase": …}`, `Value = plus − minus`.

- [ ] **Step 1: Write the failing test** `pkg/mapping/energymeter_test.go`

```go
package mapping

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "gitlab.com/bboehmke/sunny"
)

// find returns the snapshot with the given name and phase, or fails.
func find(t *testing.T, snaps []Snapshot, name, phase string) Snapshot {
    t.Helper()
    for _, s := range snaps {
        if s.Name == name && s.Labels["phase"] == phase {
            return s
        }
    }
    t.Fatalf("snapshot %s{phase=%s} not found", name, phase)
    return Snapshot{}
}

func TestMapEnergyMeterSignsPower(t *testing.T) {
    values := map[sunny.ValueID]interface{}{
        // L1 imports 172.6 W, L2 feeds in 692.9 W
        sunny.ActivePowerPlusL1:  172.6,
        sunny.ActivePowerMinusL1: 0.0,
        sunny.ActivePowerPlusL2:  0.0,
        sunny.ActivePowerMinusL2: 692.9,
        sunny.ActivePowerPlus:    15.4,
        sunny.ActivePowerMinus:   0.0,
    }
    snaps := MapEnergyMeter(values)

    assert.InDelta(t, 172.6, find(t, snaps, "active_power", "l1").Value, 0.001)
    assert.InDelta(t, -692.9, find(t, snaps, "active_power", "l2").Value, 0.001)
    assert.InDelta(t, 15.4, find(t, snaps, "active_power", "total").Value, 0.001)
    assert.Equal(t, Gauge, find(t, snaps, "active_power", "total").Type)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/mapping/... -run TestMapEnergyMeterSignsPower -v`
Expected: FAIL (undefined `MapEnergyMeter`).

- [ ] **Step 3: Implement `pkg/mapping/energymeter.go`**

```go
package mapping

import "gitlab.com/bboehmke/sunny"

type powerDef struct {
    phase       string
    plus, minus sunny.ValueID
}

var activePower = []powerDef{
    {"total", sunny.ActivePowerPlus, sunny.ActivePowerMinus},
    {"l1", sunny.ActivePowerPlusL1, sunny.ActivePowerMinusL1},
    {"l2", sunny.ActivePowerPlusL2, sunny.ActivePowerMinusL2},
    {"l3", sunny.ActivePowerPlusL3, sunny.ActivePowerMinusL3},
}

var reactivePower = []powerDef{
    {"total", sunny.ReactivePowerPlus, sunny.ReactivePowerMinus},
    {"l1", sunny.ReactivePowerPlusL1, sunny.ReactivePowerMinusL1},
    {"l2", sunny.ReactivePowerPlusL2, sunny.ReactivePowerMinusL2},
    {"l3", sunny.ReactivePowerPlusL3, sunny.ReactivePowerMinusL3},
}

var apparentPower = []powerDef{
    {"total", sunny.ApparentPowerPlus, sunny.ApparentPowerMinus},
    {"l1", sunny.ApparentPowerPlusL1, sunny.ApparentPowerMinusL1},
    {"l2", sunny.ApparentPowerPlusL2, sunny.ApparentPowerMinusL2},
    {"l3", sunny.ApparentPowerPlusL3, sunny.ApparentPowerMinusL3},
}

// MapEnergyMeter converts a raw sunny energy-meter value map into bare metric snapshots.
func MapEnergyMeter(values map[sunny.ValueID]interface{}) []Snapshot {
    var out []Snapshot
    out = append(out, signedPower(values, "active_power", activePower)...)
    out = append(out, signedPower(values, "reactive_power", reactivePower)...)
    out = append(out, signedPower(values, "apparent_power", apparentPower)...)
    return out
}

func signedPower(values map[sunny.ValueID]interface{}, name string, defs []powerDef) []Snapshot {
    var out []Snapshot
    for _, d := range defs {
        plus, okP := toFloat(values[d.plus])
        minus, okM := toFloat(values[d.minus])
        if !okP && !okM {
            continue
        }
        out = append(out, Snapshot{
            Name:   name,
            Type:   Gauge,
            Labels: map[string]string{"phase": d.phase},
            Value:  plus - minus,
        })
    }
    return out
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/mapping/... -run TestMapEnergyMeterSignsPower -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/mapping/energymeter.go pkg/mapping/energymeter_test.go
git commit -m "Map energy-meter active/reactive/apparent power as signed gauges"
```

---

### Task 11: Direct gauges (current, voltage, power factor, frequency)

**Files:**

- Modify: `pkg/mapping/energymeter.go`
- Test: `pkg/mapping/energymeter_test.go`

**Interfaces:**

- Produces additional snapshots from `MapEnergyMeter`: `current`/`voltage` (`Labels{"phase": l1|l2|l3}`), `power_factor`
  (`phase` total|l1|l2|l3), `frequency` (no `phase` label). All `Type=Gauge`.

- [ ] **Step 1: Add the failing test** (append to `energymeter_test.go`)

```go
func TestMapEnergyMeterDirectGauges(t *testing.T) {
values := map[sunny.ValueID]interface{}{
sunny.CurrentL1:        1.423,
sunny.VoltageL1:        231.07,
sunny.PowerFactor:      0.01,
sunny.PowerFactorL1:    0.642,
sunny.UtilityFrequency: 50.0,
}
snaps := MapEnergyMeter(values)

assert.InDelta(t, 1.423, find(t, snaps, "current", "l1").Value, 0.001)
assert.InDelta(t, 231.07, find(t, snaps, "voltage", "l1").Value, 0.001)
assert.InDelta(t, 0.642, find(t, snaps, "power_factor", "l1").Value, 0.001)
assert.InDelta(t, 0.01, find(t, snaps, "power_factor", "total").Value, 0.001)

// frequency has no phase label
var freq *Snapshot
for i := range snaps {
if snaps[i].Name == "frequency" {
freq = &snaps[i]
}
}
if assert.NotNil(t, freq) {
assert.InDelta(t, 50.0, freq.Value, 0.001)
_, hasPhase := freq.Labels["phase"]
assert.False(t, hasPhase)
}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/mapping/... -run TestMapEnergyMeterDirectGauges -v`
Expected: FAIL (current/voltage/etc. not produced).

- [ ] **Step 3: Extend `energymeter.go`** — add the direct-gauge table + helper, and call it from `MapEnergyMeter`.

```go
type directDef struct {
id     sunny.ValueID
name   string
labels map[string]string
}

var directGaugeDefs = []directDef{
{sunny.CurrentL1, "current", map[string]string{"phase": "l1"}},
{sunny.CurrentL2, "current", map[string]string{"phase": "l2"}},
{sunny.CurrentL3, "current", map[string]string{"phase": "l3"}},
{sunny.VoltageL1, "voltage", map[string]string{"phase": "l1"}},
{sunny.VoltageL2, "voltage", map[string]string{"phase": "l2"}},
{sunny.VoltageL3, "voltage", map[string]string{"phase": "l3"}},
{sunny.PowerFactor, "power_factor", map[string]string{"phase": "total"}},
{sunny.PowerFactorL1, "power_factor", map[string]string{"phase": "l1"}},
{sunny.PowerFactorL2, "power_factor", map[string]string{"phase": "l2"}},
{sunny.PowerFactorL3, "power_factor", map[string]string{"phase": "l3"}},
{sunny.UtilityFrequency, "frequency", map[string]string{}},
}

func directGauges(values map[sunny.ValueID]interface{}) []Snapshot {
var out []Snapshot
for _, d := range directGaugeDefs {
if f, ok := toFloat(values[d.id]); ok {
out = append(out, Snapshot{Name: d.name, Type: Gauge, Labels: d.labels, Value: f})
}
}
return out
}
```

Add to `MapEnergyMeter` before `return out`:

```go
    out = append(out, directGauges(values)...)
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/mapping/... -v`
Expected: PASS (all mapping tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/mapping/energymeter.go pkg/mapping/energymeter_test.go
git commit -m "Map energy-meter current, voltage, power factor and frequency"
```

---

### Task 12: Directional energy counters (kWh)

**Files:**

- Modify: `pkg/mapping/energymeter.go`
- Test: `pkg/mapping/energymeter_test.go`

**Interfaces:**

- Produces additional snapshots: `energy_total`/`reactive_energy_total`/`apparent_energy_total`, `Type=Counter`,
  `Labels{"phase": …, "direction": consumption|delivery}`, value = raw `Ws × (1/3 600 000)` kWh.

- [ ] **Step 1: Add the failing test** (append to `energymeter_test.go`)

```go
func TestMapEnergyMeterEnergyCountersKWh(t *testing.T) {
values := map[sunny.ValueID]interface{}{
// 24 692.866 kWh consumed on total => 24692.866 * 3.6e6 Ws
sunny.ActiveEnergyPlus:  uint64(24692.866 * 3_600_000),
sunny.ActiveEnergyMinus: uint64(17724.3048 * 3_600_000),
}
snaps := MapEnergyMeter(values)

var cons, del *Snapshot
for i := range snaps {
if snaps[i].Name != "energy_total" || snaps[i].Labels["phase"] != "total" {
continue
}
switch snaps[i].Labels["direction"] {
case "consumption":
cons = &snaps[i]
case "delivery":
del = &snaps[i]
}
}
if assert.NotNil(t, cons) {
assert.Equal(t, Counter, cons.Type)
assert.InDelta(t, 24692.866, cons.Value, 0.01)
}
if assert.NotNil(t, del) {
assert.InDelta(t, 17724.3048, del.Value, 0.01)
}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/mapping/... -run TestMapEnergyMeterEnergyCountersKWh -v`
Expected: FAIL.

- [ ] **Step 3: Extend `energymeter.go`** — add the energy table + helper and call it.

```go
const wsToKWh = 1.0 / 3_600_000.0

type energyDef struct {
id        sunny.ValueID
name      string
phase     string
direction string
}

var energyDefs = []energyDef{
{sunny.ActiveEnergyPlus, "energy_total", "total", "consumption"},
{sunny.ActiveEnergyMinus, "energy_total", "total", "delivery"},
{sunny.ActiveEnergyPlusL1, "energy_total", "l1", "consumption"},
{sunny.ActiveEnergyMinusL1, "energy_total", "l1", "delivery"},
{sunny.ActiveEnergyPlusL2, "energy_total", "l2", "consumption"},
{sunny.ActiveEnergyMinusL2, "energy_total", "l2", "delivery"},
{sunny.ActiveEnergyPlusL3, "energy_total", "l3", "consumption"},
{sunny.ActiveEnergyMinusL3, "energy_total", "l3", "delivery"},

{sunny.ReactiveEnergyPlus, "reactive_energy_total", "total", "consumption"},
{sunny.ReactiveEnergyMinus, "reactive_energy_total", "total", "delivery"},
{sunny.ReactiveEnergyPlusL1, "reactive_energy_total", "l1", "consumption"},
{sunny.ReactiveEnergyMinusL1, "reactive_energy_total", "l1", "delivery"},
{sunny.ReactiveEnergyPlusL2, "reactive_energy_total", "l2", "consumption"},
{sunny.ReactiveEnergyMinusL2, "reactive_energy_total", "l2", "delivery"},
{sunny.ReactiveEnergyPlusL3, "reactive_energy_total", "l3", "consumption"},
{sunny.ReactiveEnergyMinusL3, "reactive_energy_total", "l3", "delivery"},

{sunny.ApparentEnergyPlus, "apparent_energy_total", "total", "consumption"},
{sunny.ApparentEnergyMinus, "apparent_energy_total", "total", "delivery"},
{sunny.ApparentEnergyPlusL1, "apparent_energy_total", "l1", "consumption"},
{sunny.ApparentEnergyMinusL1, "apparent_energy_total", "l1", "delivery"},
{sunny.ApparentEnergyPlusL2, "apparent_energy_total", "l2", "consumption"},
{sunny.ApparentEnergyMinusL2, "apparent_energy_total", "l2", "delivery"},
{sunny.ApparentEnergyPlusL3, "apparent_energy_total", "l3", "consumption"},
{sunny.ApparentEnergyMinusL3, "apparent_energy_total", "l3", "delivery"},
}

func energyCounters(values map[sunny.ValueID]interface{}) []Snapshot {
var out []Snapshot
for _, d := range energyDefs {
if f, ok := toFloat(values[d.id]); ok {
out = append(out, Snapshot{
Name:   d.name,
Type:   Counter,
Labels: map[string]string{"phase": d.phase, "direction": d.direction},
Value:  f * wsToKWh,
})
}
}
return out
}
```

Add to `MapEnergyMeter` before `return out`:

```go
    out = append(out, energyCounters(values)...)
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/mapping/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/mapping/energymeter.go pkg/mapping/energymeter_test.go
git commit -m "Map energy-meter energy counters as directional kWh counters"
```

---

### Task 13: Push collector

**Files:**

- Create: `pkg/collector/collector.go`
- Test: `pkg/collector/collector_test.go`

**Interfaces:**

- Produces:

```go
func NewCollector() *Collector // implements prometheus.Collector
func (c *Collector) Describe(chan<- *prometheus.Desc) // unchecked (no-op)
func (c *Collector) Collect(chan<- prometheus.Metric)
func (c *Collector) Observe(prefix, serial string, snaps []mapping.Snapshot, constLabels map[string]string)
```

- Behaviour: `Observe` builds a `prometheus.Metric` per snapshot (`fqName = prefix + "_" + snap.Name`, const labels =
  `constLabels` merged with `snap.Labels`) and stores it in an expiring cache keyed by `serial|fqName|sortedLabels`.
  `Collect` emits every cached metric. Values older than the cache timeout expire automatically.

- [ ] **Step 1: Add the go-cache dep**

```bash
go get github.com/patrickmn/go-cache@latest
```

- [ ] **Step 2: Write the failing test** `pkg/collector/collector_test.go`

```go
package collector

import (
    "testing"

    "github.com/chr-fritz/speedwire-exporter/pkg/mapping"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/stretchr/testify/require"
)

func TestCollectorEmitsObservedMetric(t *testing.T) {
    c := NewCollector()
    reg := prometheus.NewPedanticRegistry()
    require.NoError(t, reg.Register(c))

    c.Observe("smartmeter", "1234", []mapping.Snapshot{
        {Name: "active_power", Type: mapping.Gauge, Labels: map[string]string{"phase": "l2"}, Value: -692.9},
    }, map[string]string{"meter": "grid"})

    mfs, err := reg.Gather()
    require.NoError(t, err)

    var found bool
    for _, mf := range mfs {
        if mf.GetName() != "smartmeter_active_power" {
            continue
        }
        found = true
        m := mf.GetMetric()[0]
        require.InDelta(t, -692.9, m.GetGauge().GetValue(), 0.001)
        labels := map[string]string{}
        for _, l := range m.GetLabel() {
            labels[l.GetName()] = l.GetValue()
        }
        require.Equal(t, "grid", labels["meter"])
        require.Equal(t, "l2", labels["phase"])
    }
    require.True(t, found, "smartmeter_active_power not gathered")
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./pkg/collector/... -v`
Expected: FAIL (undefined `NewCollector`).

- [ ] **Step 4: Implement `pkg/collector/collector.go`**

```go
package collector

import (
    "log/slog"
    "sort"
    "strings"
    "time"

    "github.com/chr-fritz/speedwire-exporter/pkg/mapping"
    "github.com/patrickmn/go-cache"
    "github.com/prometheus/client_golang/prometheus"
)

const defaultTimeout = 30 * time.Second

type Collector struct {
    cache *cache.Cache
    now   func() time.Time
}

func NewCollector() *Collector {
    return &Collector{
        cache: cache.New(defaultTimeout, defaultTimeout*10),
        now:   time.Now,
    }
}

// Describe leaves the collector unchecked (dynamic label sets).
func (c *Collector) Describe(chan<- *prometheus.Desc) {}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
    for _, item := range c.cache.Items() {
        if m, ok := item.Object.(prometheus.Metric); ok {
            ch <- m
        }
    }
}

func (c *Collector) Observe(prefix, serial string, snaps []mapping.Snapshot, constLabels map[string]string) {
    for _, s := range snaps {
        labels := map[string]string{}
        for k, v := range constLabels {
            labels[k] = v
        }
        for k, v := range s.Labels {
            labels[k] = v
        }

        names, values := sortedLabels(labels)
        fqName := prefix + "_" + s.Name
        desc := prometheus.NewDesc(fqName, "", names, nil)

        valueType := prometheus.GaugeValue
        if s.Type == mapping.Counter {
            valueType = prometheus.CounterValue
        }

        metric, err := prometheus.NewConstMetric(desc, valueType, s.Value, values...)
        if err != nil {
            slog.With("metric", fqName, "err", err).Warn("can not build metric")
            continue
        }
        metric = prometheus.NewMetricWithTimestamp(c.now(), metric)
        c.cache.Set(serial+"|"+fqName+"|"+strings.Join(values, ","), metric, cache.DefaultExpiration)
    }
}

func sortedLabels(labels map[string]string) (names, values []string) {
    names = make([]string, 0, len(labels))
    for k := range labels {
        names = append(names, k)
    }
    sort.Strings(names)
    values = make([]string, 0, len(labels))
    for _, k := range names {
        values = append(values, labels[k])
    }
    return names, values
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./pkg/collector/... -v`
Expected: PASS.

- [ ] **Step 6: Add an expiry test** (append to `collector_test.go`)

```go
func TestCollectorExpiresStaleMetrics(t *testing.T) {
c := NewCollector()
c.cache.Set("stale|smartmeter_x|", prometheus.MustNewConstMetric(
prometheus.NewDesc("smartmeter_x", "", nil, nil), prometheus.GaugeValue, 1), -1) // already expired

reg := prometheus.NewPedanticRegistry()
_ = reg.Register(c)
mfs, _ := reg.Gather()
for _, mf := range mfs {
require.NotEqual(t, "smartmeter_x", mf.GetName())
}
}
```

Run: `go test ./pkg/collector/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/collector go.mod go.sum
git commit -m "Add push-based expiring Prometheus collector"
```

---

### Task 14: Speedwire listener → collector, wired into `run`

**Files:**

- Create: `pkg/speedwire/listener.go`
- Modify: `cmd/run.go`

**Interfaces:**

- Consumes: `config.Config`, `collector.Collector`, `mapping.MapEnergyMeter`, `speedwire` discovery primitives.
- Produces:

```go
type Listener struct { /* … */ }
func NewListener(cfg *config.Config, col *collector.Collector) *Listener
func (l *Listener) Run(ctx context.Context) // discovers, then loops reading + Observe per configured device
```

- [ ] **Step 1: Implement `pkg/speedwire/listener.go`**

```go
package speedwire

import (
    "context"
    "log/slog"
    "strconv"
    "sync"
    "time"

    "github.com/chr-fritz/speedwire-exporter/pkg/collector"
    "github.com/chr-fritz/speedwire-exporter/pkg/config"
    "github.com/chr-fritz/speedwire-exporter/pkg/mapping"
    "gitlab.com/bboehmke/sunny"
)

type Listener struct {
    cfg          *config.Config
    col          *collector.Collector
    seenUnmapped sync.Map // sunny.ValueID -> struct{}
}

func NewListener(cfg *config.Config, col *collector.Collector) *Listener {
    return &Listener{cfg: cfg, col: col}
}

func (l *Listener) Run(ctx context.Context) {
    conn, err := sunny.NewConnection(l.cfg.Interface)
    if err != nil {
        slog.With("err", err).Error("can not open speedwire connection")
        return
    }

    devs := make(chan *sunny.Device)
    go conn.DiscoverDevices(ctx, devs, l.cfg.Discovery.Password)

    ticker := time.NewTicker(l.cfg.FetchInterval)
    defer ticker.Stop()
    known := map[uint32]*sunny.Device{}

    for {
        select {
        case dev := <-devs:
            if dev != nil {
                known[dev.SerialNumber()] = dev
            }
        case <-ticker.C:
            for serial, dev := range known {
                l.read(ctx, serial, dev)
            }
        case <-ctx.Done():
            return
        }
    }
}

func (l *Listener) read(ctx context.Context, serial uint32, dev *sunny.Device) {
    labels, ok := l.cfg.LabelsFor(serial)
    if !ok {
        slog.With("serial", serial).Debug("skipping unconfigured device")
        return
    }
    values, err := dev.GetValuesCtx(ctx)
    if err != nil {
        slog.With("serial", serial, "err", err).Warn("can not read values")
        return
    }

    if dev.IsEnergyMeter() {
        l.logUnmapped(values)
        l.col.Observe(l.cfg.Metrics.EnergyMeterPrefix, strconv.FormatUint(uint64(serial), 10),
            mapping.MapEnergyMeter(values), labels)
    }
    // Inverter mapping is added in the P2 plan.
}

// logUnmapped logs values without a mapping once per ValueID at info level.
func (l *Listener) logUnmapped(values map[sunny.ValueID]interface{}) {
    for id := range values {
        if !mapping.IsMappedEnergyMeter(id) {
            if _, loaded := l.seenUnmapped.LoadOrStore(id, struct{}{}); !loaded {
                slog.With("valueId", id, "description", sunny.GetValueInfo(id).Description).
                    Info("unmapped speedwire value")
            }
        }
    }
}
```

- [ ] **Step 2: Add `IsMappedEnergyMeter` to the mapping package** (`pkg/mapping/energymeter.go`)

```go
// mappedEnergyMeterIDs is the set of ValueIDs the energy-meter mapping consumes.
var mappedEnergyMeterIDs = func () map[sunny.ValueID]struct{} {
m := map[sunny.ValueID]struct{}{}
for _, d := range activePower {
m[d.plus] = struct{}{}
m[d.minus] = struct{}{}
}
for _, d := range reactivePower {
m[d.plus] = struct{}{}
m[d.minus] = struct{}{}
}
for _, d := range apparentPower {
m[d.plus] = struct{}{}
m[d.minus] = struct{}{}
}
for _, d := range directGaugeDefs {
m[d.id] = struct{}{}
}
for _, d := range energyDefs {
m[d.id] = struct{}{}
}
return m
}()

// IsMappedEnergyMeter reports whether the energy-meter mapping consumes the value.
func IsMappedEnergyMeter(id sunny.ValueID) bool {
_, ok := mappedEnergyMeterIDs[id]
return ok
}
```

- [ ] **Step 3: Write a test** `pkg/mapping/energymeter_test.go` (append) for the mapped-set helper

```go
func TestIsMappedEnergyMeter(t *testing.T) {
assert.True(t, IsMappedEnergyMeter(sunny.ActivePowerPlus))
assert.True(t, IsMappedEnergyMeter(sunny.UtilityFrequency))
// PowerS1 is a DC-string value an energy meter never reports and the mapping does not consume.
assert.False(t, IsMappedEnergyMeter(sunny.PowerS1))
}
```

Run: `go test ./pkg/mapping/... -run TestIsMappedEnergyMeter -v`
Expected: PASS.

- [ ] **Step 4: Wire the listener into `cmd/run.go`** — after registering the metrics handler, before `httpServer.Run`:

Add imports `github.com/chr-fritz/speedwire-exporter/pkg/collector` and
`github.com/chr-fritz/speedwire-exporter/pkg/speedwire`, then insert:

```go
    col := collector.NewCollector()
if err = registry.Register(col); err != nil {
slog.With("err", err).Error("can not register collector")
return
}
listener := speedwire.NewListener(&cfg, col)
go listener.Run(ctx)
```

- [ ] **Step 5: Build and run the full test suite**

Run: `go build ./... && go test ./... -v`
Expected: build succeeds; all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/speedwire/listener.go pkg/mapping/energymeter.go pkg/mapping/energymeter_test.go cmd/run.go
git commit -m "Add speedwire listener feeding the collector and wire it into run"
```

---

### Task 15: Optional `info` metric (gated by `metrics.info`)

**Files:**

- Modify: `pkg/mapping/energymeter.go`, `pkg/speedwire/listener.go`
- Test: `pkg/mapping/energymeter_test.go`

**Interfaces:**

- Produces: `func EnergyMeterInfo(values map[sunny.ValueID]interface{}) (Snapshot, bool)` returning an `info` snapshot
  (`Name="info"`, `Type=Gauge`, `Value=1`, `Labels{"software_version": …}`) when `SoftwareVersion` is present.

- [ ] **Step 1: Write the failing test** (append to `energymeter_test.go`)

```go
func TestEnergyMeterInfo(t *testing.T) {
s, ok := EnergyMeterInfo(map[sunny.ValueID]interface{}{
sunny.SoftwareVersion: "1.2.4.R",
})
assert.True(t, ok)
assert.Equal(t, "info", s.Name)
assert.Equal(t, float64(1), s.Value)
assert.Equal(t, "1.2.4.R", s.Labels["software_version"])

_, ok = EnergyMeterInfo(map[sunny.ValueID]interface{}{})
assert.False(t, ok)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/mapping/... -run TestEnergyMeterInfo -v`
Expected: FAIL.

- [ ] **Step 3: Implement `EnergyMeterInfo`** in `energymeter.go`

```go
import "fmt" // add to existing imports

// EnergyMeterInfo returns a value-1 info snapshot carrying the software version, if present.
func EnergyMeterInfo(values map[sunny.ValueID]interface{}) (Snapshot, bool) {
v, ok := values[sunny.SoftwareVersion]
if !ok {
return Snapshot{}, false
}
return Snapshot{
Name:   "info",
Type:   Gauge,
Labels: map[string]string{"software_version": fmt.Sprintf("%v", v)},
Value:  1,
}, true
}
```

Also add `SoftwareVersion` to `mappedEnergyMeterIDs` so it is not logged as unmapped:

```go
    m[sunny.SoftwareVersion] = struct{}{}
```

(add this line inside the `mappedEnergyMeterIDs` initialiser before `return m`).

- [ ] **Step 4: Emit it from the listener when enabled** — in `listener.go` `read`, inside the `IsEnergyMeter` branch
  after the `Observe` call:

```go
        if l.cfg.Metrics.Info {
if info, ok := mapping.EnergyMeterInfo(values); ok {
l.col.Observe(l.cfg.Metrics.EnergyMeterPrefix, strconv.FormatUint(uint64(serial), 10),
[]mapping.Snapshot{info}, labels)
}
}
```

- [ ] **Step 5: Run tests and build**

Run: `go test ./... && go build ./...`
Expected: PASS + build succeeds.

- [ ] **Step 6: Commit**

```bash
git add pkg/mapping/energymeter.go pkg/mapping/energymeter_test.go pkg/speedwire/listener.go
git commit -m "Add optional software-version info metric gated by metrics.info"
```

---

## P2 — Inverters

Inverters are **not signed** (they generate). Metric family `sunny_inverter_*`
(configurable via `metrics.inverterPrefix`). Inverter energy uses sunny's ready
`*KWh` value IDs — **no `Ws→kWh` conversion**. Reuses the collector and listener from P0/P1 unchanged except for a new
device branch.

### Task 16: Inverter core mapping (power, voltage, current, frequency, temperature)

**Files:**

- Create: `pkg/mapping/inverter.go`
- Test: `pkg/mapping/inverter_test.go`

**Interfaces:**

- Produces: `func MapInverter(values map[sunny.ValueID]interface{}) []Snapshot` (extended in Task 17);
  `func gaugesFrom(values map[sunny.ValueID]interface{}, defs []directDef) []Snapshot` (shared gauge helper).
- Snapshots have `Type=Gauge`; `power`/`voltage`/`current` carry `Labels{"side": AC|DC, "phase": …}`; `frequency`/
  `temperature`/`power_max` carry no extra labels.

- [ ] **Step 1: Write the failing test** `pkg/mapping/inverter_test.go`

```go
package mapping

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "gitlab.com/bboehmke/sunny"
)

func findLabeled(t *testing.T, snaps []Snapshot, name string, want map[string]string) Snapshot {
    t.Helper()
outer:
    for _, s := range snaps {
        if s.Name != name {
            continue
        }
        for k, v := range want {
            if s.Labels[k] != v {
                continue outer
            }
        }
        return s
    }
    t.Fatalf("snapshot %s %v not found", name, want)
    return Snapshot{}
}

func TestMapInverterCore(t *testing.T) {
    values := map[sunny.ValueID]interface{}{
        sunny.ActivePowerPlus:   3200.0,
        sunny.PowerS1:           1600.0,
        sunny.VoltageL1:         231.0,
        sunny.CurrentS1:         5.2,
        sunny.UtilityFrequency:  50.0,
        sunny.DeviceTemperature: 42.5,
    }
    snaps := MapInverter(values)

    assert.InDelta(t, 3200.0, findLabeled(t, snaps, "power", map[string]string{"side": "AC", "phase": "total"}).Value, 0.001)
    assert.InDelta(t, 1600.0, findLabeled(t, snaps, "power", map[string]string{"side": "DC", "phase": "1"}).Value, 0.001)
    assert.InDelta(t, 231.0, findLabeled(t, snaps, "voltage", map[string]string{"side": "AC", "phase": "l1"}).Value, 0.001)
    assert.InDelta(t, 5.2, findLabeled(t, snaps, "current", map[string]string{"side": "DC", "phase": "1"}).Value, 0.001)
    assert.Equal(t, Gauge, findLabeled(t, snaps, "power", map[string]string{"side": "AC", "phase": "total"}).Type)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/mapping/... -run TestMapInverterCore -v`
Expected: FAIL (undefined `MapInverter`).

- [ ] **Step 3: Implement `pkg/mapping/inverter.go`**

```go
package mapping

import "gitlab.com/bboehmke/sunny"

var inverterGaugeDefs = []directDef{
    // AC power
    {sunny.ActivePowerPlus, "power", map[string]string{"side": "AC", "phase": "total"}},
    {sunny.ActivePowerPlusL1, "power", map[string]string{"side": "AC", "phase": "l1"}},
    {sunny.ActivePowerPlusL2, "power", map[string]string{"side": "AC", "phase": "l2"}},
    {sunny.ActivePowerPlusL3, "power", map[string]string{"side": "AC", "phase": "l3"}},
    // DC power (strings)
    {sunny.PowerS1, "power", map[string]string{"side": "DC", "phase": "1"}},
    {sunny.PowerS2, "power", map[string]string{"side": "DC", "phase": "2"}},
    // AC voltage
    {sunny.VoltageL1, "voltage", map[string]string{"side": "AC", "phase": "l1"}},
    {sunny.VoltageL2, "voltage", map[string]string{"side": "AC", "phase": "l2"}},
    {sunny.VoltageL3, "voltage", map[string]string{"side": "AC", "phase": "l3"}},
    {sunny.VoltageL1L2, "voltage", map[string]string{"side": "AC", "phase": "l1l2"}},
    {sunny.VoltageL2L3, "voltage", map[string]string{"side": "AC", "phase": "l2l3"}},
    {sunny.VoltageL3L1, "voltage", map[string]string{"side": "AC", "phase": "l3l1"}},
    // DC voltage
    {sunny.VoltageS1, "voltage", map[string]string{"side": "DC", "phase": "1"}},
    {sunny.VoltageS2, "voltage", map[string]string{"side": "DC", "phase": "2"}},
    // AC current
    {sunny.CurrentL1, "current", map[string]string{"side": "AC", "phase": "l1"}},
    {sunny.CurrentL2, "current", map[string]string{"side": "AC", "phase": "l2"}},
    {sunny.CurrentL3, "current", map[string]string{"side": "AC", "phase": "l3"}},
    // DC current
    {sunny.CurrentS1, "current", map[string]string{"side": "DC", "phase": "1"}},
    {sunny.CurrentS2, "current", map[string]string{"side": "DC", "phase": "2"}},
    // misc
    {sunny.UtilityFrequency, "frequency", map[string]string{}},
    {sunny.DeviceTemperature, "temperature", map[string]string{}},
    {sunny.ActivePowerMax, "power_max", map[string]string{}},
}

// gaugesFrom builds gauge snapshots for every present value in defs.
func gaugesFrom(values map[sunny.ValueID]interface{}, defs []directDef) []Snapshot {
    var out []Snapshot
    for _, d := range defs {
        if f, ok := toFloat(values[d.id]); ok {
            out = append(out, Snapshot{Name: d.name, Type: Gauge, Labels: d.labels, Value: f})
        }
    }
    return out
}

// MapInverter converts a raw sunny inverter value map into bare metric snapshots.
func MapInverter(values map[sunny.ValueID]interface{}) []Snapshot {
    var out []Snapshot
    out = append(out, gaugesFrom(values, inverterGaugeDefs)...)
    return out
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/mapping/... -run TestMapInverterCore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/mapping/inverter.go pkg/mapping/inverter_test.go
git commit -m "Map inverter power, voltage, current, frequency and temperature"
```

---

### Task 17: Inverter energy, hybrid metering and battery

**Files:**

- Modify: `pkg/mapping/inverter.go`
- Test: `pkg/mapping/inverter_test.go`

**Interfaces:**

- Produces additional snapshots from `MapInverter`:
    - kWh counters (`Type=Counter`, value used directly from `*KWh` IDs): `energy{interval=total}`, `pv_energy`,
      `grid_energy{direction=export|import,interval=total}`, `consumption_energy`, `self_consumption_energy`,
      `battery_energy{direction=charge|discharge}`.
    - non-monotonic day gauges (`Type=Gauge`): `energy_today`, `grid_energy_today{direction}`.
    - hybrid power gauges: `pv_power`, `grid_power{direction}`, `consumption_power`, `self_consumption_power`.
    - battery gauges: `battery_charge`, `battery_temperature`, `battery_voltage`, `battery_current`,
      `battery_charge_cycles`.

- [ ] **Step 1: Add the failing test** (append to `inverter_test.go`)

```go
func TestMapInverterEnergyAndHybrid(t *testing.T) {
values := map[sunny.ValueID]interface{}{
sunny.ActiveEnergyPlusKWh: 12345.6, // already kWh (no conversion)
sunny.PvPower:             4100.0,
sunny.GridPowerExport:     2500.0,
sunny.BatteryCharge:       87.0,
sunny.GridEnergyExportKWh: 987.6,
}
snaps := MapInverter(values)

energy := findLabeled(t, snaps, "energy", map[string]string{"interval": "total"})
assert.Equal(t, Counter, energy.Type)
assert.InDelta(t, 12345.6, energy.Value, 0.01) // NOT divided by 3.6e6

assert.InDelta(t, 4100.0, findLabeled(t, snaps, "pv_power", map[string]string{}).Value, 0.001)
assert.InDelta(t, 2500.0, findLabeled(t, snaps, "grid_power", map[string]string{"direction": "export"}).Value, 0.001)
assert.InDelta(t, 87.0, findLabeled(t, snaps, "battery_charge", map[string]string{}).Value, 0.001)

ge := findLabeled(t, snaps, "grid_energy", map[string]string{"direction": "export", "interval": "total"})
assert.Equal(t, Counter, ge.Type)
assert.InDelta(t, 987.6, ge.Value, 0.01)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/mapping/... -run TestMapInverterEnergyAndHybrid -v`
Expected: FAIL.

- [ ] **Step 3: Extend `pkg/mapping/inverter.go`** — add tables, the `kwhDef`/`kwhCounters` helper, and append to
  `MapInverter`.

```go
type kwhDef struct {
id     sunny.ValueID
name   string
labels map[string]string
}

// inverterEnergyCounters use sunny's *KWh value IDs directly (already divided).
var inverterEnergyCounters = []kwhDef{
{sunny.ActiveEnergyPlusKWh, "energy", map[string]string{"interval": "total"}},
{sunny.PvEnergyTotalKWh, "pv_energy", map[string]string{}},
{sunny.GridEnergyExportKWh, "grid_energy", map[string]string{"direction": "export", "interval": "total"}},
{sunny.GridEnergyImportKWh, "grid_energy", map[string]string{"direction": "import", "interval": "total"}},
{sunny.ConsumptionEnergyKWh, "consumption_energy", map[string]string{}},
{sunny.SelfConsumptionKWh, "self_consumption_energy", map[string]string{}},
{sunny.BatteryEnergyChargeKWh, "battery_energy", map[string]string{"direction": "charge"}},
{sunny.BatteryEnergyDischargeKWh, "battery_energy", map[string]string{"direction": "discharge"}},
}

// non-monotonic daily counters → exported as gauges
var inverterEnergyGauges = []directDef{
{sunny.ActiveEnergyPlusTodayKWh, "energy_today", map[string]string{}},
{sunny.GridEnergyExportDayKWh, "grid_energy_today", map[string]string{"direction": "export"}},
{sunny.GridEnergyImportDayKWh, "grid_energy_today", map[string]string{"direction": "import"}},
}

var inverterHybridPower = []directDef{
{sunny.PvPower, "pv_power", map[string]string{}},
{sunny.GridPowerExport, "grid_power", map[string]string{"direction": "export"}},
{sunny.GridPowerImport, "grid_power", map[string]string{"direction": "import"}},
{sunny.ConsumptionPower, "consumption_power", map[string]string{}},
{sunny.SelfConsumptionPower, "self_consumption_power", map[string]string{}},
}

var inverterBattery = []directDef{
{sunny.BatteryCharge, "battery_charge", map[string]string{}},
{sunny.BatteryTemperature, "battery_temperature", map[string]string{}},
{sunny.BatteryVoltage, "battery_voltage", map[string]string{}},
{sunny.BatteryCurrent, "battery_current", map[string]string{}},
{sunny.BatteryChargeCycles, "battery_charge_cycles", map[string]string{}},
}

func kwhCounters(values map[sunny.ValueID]interface{}, defs []kwhDef) []Snapshot {
var out []Snapshot
for _, d := range defs {
if f, ok := toFloat(values[d.id]); ok {
out = append(out, Snapshot{Name: d.name, Type: Counter, Labels: d.labels, Value: f})
}
}
return out
}
```

Append to `MapInverter` before `return out`:

```go
    out = append(out, gaugesFrom(values, inverterHybridPower)...)
out = append(out, kwhCounters(values, inverterEnergyCounters)...)
out = append(out, gaugesFrom(values, inverterEnergyGauges)...)
out = append(out, gaugesFrom(values, inverterBattery)...)
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/mapping/... -v`
Expected: PASS (all mapping tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/mapping/inverter.go pkg/mapping/inverter_test.go
git commit -m "Map inverter energy, hybrid metering and battery values"
```

---

### Task 18: Wire inverters into the listener

**Files:**

- Modify: `pkg/mapping/inverter.go`, `pkg/speedwire/listener.go`
- Test: `pkg/mapping/inverter_test.go`

**Interfaces:**

- Produces: `func IsMappedInverter(id sunny.ValueID) bool`;
  `func InverterInfo(values map[sunny.ValueID]interface{}) (Snapshot, bool)` (`Name="info"`, `Value=1`, labels
  `software_version`/`device_name` when present).
- The listener's `read` now handles both device types and logs unmapped values once per ValueID using the
  type-appropriate predicate.

- [ ] **Step 1: Write the failing test** (append to `inverter_test.go`)

```go
func TestIsMappedInverterAndInfo(t *testing.T) {
assert.True(t, IsMappedInverter(sunny.PowerS1))
assert.True(t, IsMappedInverter(sunny.PvPower))
assert.False(t, IsMappedInverter(sunny.ReactiveEnergyPlusL1))

s, ok := InverterInfo(map[sunny.ValueID]interface{}{sunny.SoftwareVersion: "3.10.24.R"})
assert.True(t, ok)
assert.Equal(t, "info", s.Name)
assert.Equal(t, "3.10.24.R", s.Labels["software_version"])
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/mapping/... -run TestIsMappedInverterAndInfo -v`
Expected: FAIL.

- [ ] **Step 3: Implement `IsMappedInverter` and `InverterInfo`** in `inverter.go`

```go
import "fmt" // add to existing imports

var mappedInverterIDs = func () map[sunny.ValueID]struct{} {
m := map[sunny.ValueID]struct{}{}
for _, d := range inverterGaugeDefs {
m[d.id] = struct{}{}
}
for _, d := range inverterEnergyGauges {
m[d.id] = struct{}{}
}
for _, d := range inverterHybridPower {
m[d.id] = struct{}{}
}
for _, d := range inverterBattery {
m[d.id] = struct{}{}
}
for _, d := range inverterEnergyCounters {
m[d.id] = struct{}{}
}
m[sunny.SoftwareVersion] = struct{}{}
m[sunny.DeviceName] = struct{}{}
return m
}()

// IsMappedInverter reports whether the inverter mapping consumes the value.
func IsMappedInverter(id sunny.ValueID) bool {
_, ok := mappedInverterIDs[id]
return ok
}

// InverterInfo returns a value-1 info snapshot carrying software version / device name, if present.
func InverterInfo(values map[sunny.ValueID]interface{}) (Snapshot, bool) {
labels := map[string]string{}
if v, ok := values[sunny.SoftwareVersion]; ok {
labels["software_version"] = fmt.Sprintf("%v", v)
}
if v, ok := values[sunny.DeviceName]; ok {
labels["device_name"] = fmt.Sprintf("%v", v)
}
if len(labels) == 0 {
return Snapshot{}, false
}
return Snapshot{Name: "info", Type: Gauge, Labels: labels, Value: 1}, true
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/mapping/... -run TestIsMappedInverterAndInfo -v`
Expected: PASS.

- [ ] **Step 5: Replace `read` and `logUnmapped` in `pkg/speedwire/listener.go`** with a version that handles both
  device types

```go
func (l *Listener) read(ctx context.Context, serial uint32, dev *sunny.Device) {
labels, ok := l.cfg.LabelsFor(serial)
if !ok {
slog.With("serial", serial).Debug("skipping unconfigured device")
return
}
values, err := dev.GetValuesCtx(ctx)
if err != nil {
slog.With("serial", serial, "err", err).Warn("can not read values")
return
}
serialStr := strconv.FormatUint(uint64(serial), 10)

if dev.IsEnergyMeter() {
l.logUnmapped(values, mapping.IsMappedEnergyMeter)
l.col.Observe(l.cfg.Metrics.EnergyMeterPrefix, serialStr, mapping.MapEnergyMeter(values), labels)
if l.cfg.Metrics.Info {
if info, ok := mapping.EnergyMeterInfo(values); ok {
l.col.Observe(l.cfg.Metrics.EnergyMeterPrefix, serialStr, []mapping.Snapshot{info}, labels)
}
}
return
}

l.logUnmapped(values, mapping.IsMappedInverter)
l.col.Observe(l.cfg.Metrics.InverterPrefix, serialStr, mapping.MapInverter(values), labels)
if l.cfg.Metrics.Info {
if info, ok := mapping.InverterInfo(values); ok {
l.col.Observe(l.cfg.Metrics.InverterPrefix, serialStr, []mapping.Snapshot{info}, labels)
}
}
}

// logUnmapped logs values the given predicate does not cover, once per ValueID at info level.
func (l *Listener) logUnmapped(values map[sunny.ValueID]interface{}, isMapped func (sunny.ValueID) bool) {
for id := range values {
if !isMapped(id) {
if _, loaded := l.seenUnmapped.LoadOrStore(id, struct{}{}); !loaded {
slog.With("valueId", id, "description", sunny.GetValueInfo(id).Description).
Info("unmapped speedwire value")
}
}
}
}
```

(Delete the old single-branch `read`/`logUnmapped` from Task 14/15.)

- [ ] **Step 6: Build and run the full suite**

Run: `go build ./... && go test ./... -v`
Expected: build succeeds; all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/mapping/inverter.go pkg/mapping/inverter_test.go pkg/speedwire/listener.go
git commit -m "Wire inverter mapping into the speedwire listener with info metric"
```

---

## Self-Review notes (for the implementer)

- After Task 14 the exporter exposes `smartmeter_*` metrics for any configured energy meter on the network. Verify
  end-to-end against real hardware with `go run . run --config <file>` and
  `curl localhost:8080/metrics | grep smartmeter_`.
- If the installed `sunny` version's `Connection`/`Device` API differs from the snippets (they were written against the
  branch `main-old` PoC), adjust the calls in `pkg/speedwire/discover.go` and `listener.go`; the mapping and collector
  packages are independent of that API surface (they only use `sunny.ValueID` constants).
- All `sunny.*` inverter ValueIDs used in Tasks 16–18 (incl. the `*KWh` aliases and hybrid values) require **sunny ≥
  0.17.0**. `go build` fails loudly if an older version is pinned — run `go get -u gitlab.com/bboehmke/sunny` in that
  case.
- After Task 18 the exporter exposes `sunny_inverter_*` for configured speedwire inverters. Verify against real
  hardware; note `sunny_island_*` (battery, richer) stays with the modbus-exporter and Bluetooth-only inverters stay
  with `sbfspot`.
- Not in this plan (future): an alive/liveness watchdog based on last-Observe time, and P3 deployment into the
  `kampenwand` repo.

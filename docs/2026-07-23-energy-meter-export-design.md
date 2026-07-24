# Design: SMA Energy Meter Export

**Datum:** 2026-07-23 **Repo:** `github.com/chr-fritz/speedwire-exporter`

## Ziel

Der Speedwire-Exporter liest die vom SMA Energy Meter (Home Manager) am **Netzanschlusspunkt** per Speedwire-Multicast
gesendeten Werte und exportiert sie als Prometheus-Metriken. Er ersetzt die bisherige Kette
`images/sma-em` (Python-Daemon) → MQTT → `mqtt2prometheus/sma-em` durch einen einzigen nativen Go-Exporter.

Der Energy Meter misst dreiphasig und **bidirektional** (Bezug + Einspeisung). Native Go-Verarbeitung erlaubt sauber
**signierte** Leistungswerte statt eines
`direction`-Labels (Einspeisung negativ) — konsistent mit den übrigen Metriken.

## Anspruch: allgemeiner, veröffentlichbarer Exporter

Der Exporter wird **generisch** gebaut, nicht auf das Kampenwand-Setup zugeschnitten — er soll veröffentlicht werden
können. Konsequenzen:

- **Beide Gerätetypen** (Energy Meter *und* Wechselrichter) werden unterstützt; der komplette sunny-Wertesatz wird
  abgedeckt. Hintergrund: die vorhandenen Wechselrichter werden künftig durch SMA STPH-X ersetzt, die per Speedwire
  weitere (in der Lib hinterlegte) Werte liefern — die sollen dann automatisch mit erfasst werden.
- **Keine fallspezifischen Hardcodings.** Metrik- *Namen* sind fix in Go, Label- *Werte* (z.B. `meter="grid"`) kommen
  ausschließlich aus der Config.
- **Was exportiert wird, bestimmt die Config**: exportiert werden die Werte der dort registrierten Devices. Ein Gerät
  ohne Config-Eintrag wird nicht exportiert (aber im Readout sichtbar, s.u.).

### Abgrenzung zum bestehenden Setup

- `sbfspot` (mqtt2prometheus, `sunny_inverter_*`) bleibt bestehen — es bedient die **Bluetooth-only** Wechselrichter,
  die dieser Exporter gar nicht sieht (kein Speedwire) → keine Kollision. Bei Migration auf STPH-X übernimmt dieser
  Exporter deren `sunny_inverter_*`-Metriken.
- `sunny_island_*` (Batterie: SoC, Kapazität, Zyklen) bleibt beim
  `modbus-exporter` (liefert dort mehr).

## Bibliothek

`gitlab.com/bboehmke/sunny` (dieselbe Lib wie evcc), in der **aktuellsten Version** wie alle übrigen Dependencies
(`go get -u -t` + `go mod tidy`). Sie liefert die EM-Werte bereits skaliert über
`device.GetValuesCtx() → map[sunny.ValueID]interface{}`:

- Leistung `Active/Reactive/Apparent Power Plus/Minus` (Summe + L1/L2/L3):
  Einheit **W / var / VA** (float64).
- Energie `Active/Reactive/Apparent Energy Plus/Minus` (Summe + L1/L2/L3):
  Einheit **Ws / vars / VAs**, roh als **uint64**.
- `CurrentL1..3` (A), `VoltageL1..3` (V), `PowerFactor(+L1..3)`,
  `UtilityFrequency` (Hz), `SoftwareVersion`.

„Plus" = Bezug (OBIS x.4.0), „Minus" = Einspeisung (OBIS 2.x.0 …).

## Metrik-Mapping (Energy Meter)

Metrik-Familie **`smartmeter_*`** (snake_case, deckungsgleich mit dem
`modbus-exporter`), Konstant-Label **`meter="grid"`** (aus Config je Serial).

### Signierte Gauges — Einspeisung negativ, Label `phase ∈ {total,l1,l2,l3}`

| Metrik                      | Einheit | Berechnung                               |
|-----------------------------|---------|------------------------------------------|
| `smartmeter_active_power`   | W       | `ActivePowerPlus − ActivePowerMinus`     |
| `smartmeter_reactive_power` | var     | `ReactivePowerPlus − ReactivePowerMinus` |
| `smartmeter_apparent_power` | VA      | `ApparentPowerPlus − ApparentPowerMinus` |

### Direkte Gauges

| Metrik                    | Einheit | Quelle             | Label                |
|---------------------------|---------|--------------------|----------------------|
| `smartmeter_current`      | A       | `CurrentLx`        | phase=l1/l2/l3       |
| `smartmeter_voltage`      | V       | `VoltageLx`        | phase=l1/l2/l3       |
| `smartmeter_power_factor` | —       | `PowerFactor(Lx)`  | phase=total/l1/l2/l3 |
| `smartmeter_frequency`    | Hz      | `UtilityFrequency` | —                    |

### Directional Counters — `direction ∈ {consumption,delivery}`, `phase`, Einheit **kWh** (`Ws ÷ 3 600 000`)

| Metrik                             | Quelle                                            |
|------------------------------------|---------------------------------------------------|
| `smartmeter_energy_total`          | `ActiveEnergyPlus→consumption`, `…Minus→delivery` |
| `smartmeter_reactive_energy_total` | `ReactiveEnergy…`                                 |
| `smartmeter_apparent_energy_total` | `ApparentEnergy…`                                 |

`direction`-Werte `consumption`/`delivery` decken sich mit den bestehenden Recording Rules
`smartMeter:energy:grid:consumption`/`:delivery`.

**Signierung nur für Gauges** — ein Prometheus-Counter muss monoton sein, daher bleiben Bezug/Einspeisung als getrennte,
richtungsgelabelte Counter.

## Metrik-Mapping (Wechselrichter)

Metrik-Familie **`sunny_inverter_*`** (kompatibel zum bestehenden `sbfspot`, damit STPH-X später nahtlos übernommen
werden). Deckt den vollständigen sunny-Inverter-Wertesatz ab. Wechselrichter liefern nur Erzeugung → **keine
Signierung** der Leistung.

| Metrik                               | Typ     | Einheit | Quelle (sunny)                                  | Labels                    |
|--------------------------------------|---------|---------|-------------------------------------------------|---------------------------|
| `sunny_inverter_power`               | gauge   | W       | `ActivePowerPlus(L1..3)` / `PowerS1,S2`         | side=AC/DC, phase         |
| `sunny_inverter_power_max`           | gauge   | W       | `ActivePowerMax`                                | —                         |
| `sunny_inverter_voltage`             | gauge   | V       | `VoltageL1..3` / `VoltageS1,S2`                 | side, phase               |
| `sunny_inverter_current`             | gauge   | A       | `CurrentL1..3` / `CurrentS1,S2`                 | side, phase               |
| `sunny_inverter_frequency`           | gauge   | Hz      | `UtilityFrequency`                              | —                         |
| `sunny_inverter_energy`              | counter | kWh     | `ActiveEnergyPlus` / `…Minus` (Ws÷3.6e6)        | interval=total, direction |
| `sunny_inverter_energy_today`        | gauge   | kWh     | `ActiveEnergyPlusToday` (nicht monoton → gauge) | —                         |
| `sunny_inverter_temperature`         | gauge   | °C      | `DeviceTemperature`                             | —                         |
| `sunny_inverter_battery_charge`      | gauge   | %       | `BatteryCharge`                                 | —                         |
| `sunny_inverter_battery_temperature` | gauge   | °C      | `BatteryTemperature`                            | —                         |
| `sunny_inverter_operating_time`      | counter | s       | `TimeOperating` / `TimeFeed`                    | kind=operating/feed       |
| `sunny_inverter_status`              | gauge   | —       | `DeviceStatus` / `DeviceGridRelay`              | kind                      |

`DeviceName`/`DeviceClass`/`DeviceType`/`SoftwareVersion` als
`sunny_inverter_info`-Metrik (Label-Träger, Wert 1) — nur bei `metrics.info: true`. Analog liefert auch der EM
`SoftwareVersion` → `smartmeter_info` (gleicher Schalter).

### Hybrid-Wechselrichter (ab sunny 0.17.0)

Der Inverter-Wertesatz wurde in 0.17.0 für Hybrid-WR (z.B. STPH-X) erweitert — diese Werte werden mit abgedeckt:

| Gruppe           | Metrik (Vorschlag)                                                       | Typ           | Einheit | Quelle (sunny)                                          |
|------------------|--------------------------------------------------------------------------|---------------|---------|---------------------------------------------------------|
| Momentanleistung | `sunny_inverter_pv_power`                                                | gauge         | W       | `PvPower`                                               |
|                  | `sunny_inverter_grid_power` (Label `direction=export/import`)            | gauge         | W       | `GridPowerExport/Import`                                |
|                  | `sunny_inverter_consumption_power`                                       | gauge         | W       | `ConsumptionPower`                                      |
|                  | `sunny_inverter_self_consumption_power`                                  | gauge         | W       | `SelfConsumptionPower`                                  |
| Energie (kWh)    | `sunny_inverter_pv_energy`                                               | counter       | kWh     | `PvEnergyTotalKWh`                                      |
|                  | `sunny_inverter_grid_energy` (`direction`, `interval=total/today`)       | counter       | kWh     | `GridEnergy(Export/Import)(Day)KWh`                     |
|                  | `sunny_inverter_consumption_energy`                                      | counter       | kWh     | `ConsumptionEnergyKWh`                                  |
|                  | `sunny_inverter_self_consumption_energy`                                 | counter       | kWh     | `SelfConsumptionKWh`                                    |
| Batterie         | `sunny_inverter_battery_energy` (`direction=charge/discharge`)           | counter       | kWh     | `BatteryEnergy(Charge/Discharge)KWh`                    |
|                  | `sunny_inverter_battery_voltage`/`_current`/`_charge_cycles`             | gauge         | V/A/—   | `BatteryVoltage/Current/ChargeCycles`                   |
| Spannung         | `sunny_inverter_voltage` (`kind=line-line`, `phase=l1l2…`)               | gauge         | V       | `VoltageL1L2/L2L3/L3L1`                                 |
| Limits/Zeiten    | `sunny_inverter_power_limit`, `_wait_feed_in_time`, `_grid_failure_time` | gauge/counter | W/s     | `ActivePowerLimit`, `WaitTimeFeedIn`, `TimeGridFailure` |

Für Inverter-Energie werden die **fertigen `*KWh`-ValueIDs** genutzt (sunny rechnet Ws→kWh selbst); nur die EM-Energie
wird selbst umgerechnet.

**Nicht gemappte, aber gelieferte Werte** werden **nur beim ersten Vorkommen**
(je `ValueID`, dedupliziert) geloggt — max. auf `info`-Level, nicht wiederholt. Sie erscheinen zudem im Readout — so
lässt sich das Mapping bei neuen sunny-Werten gezielt erweitern, ohne den Log zuzuspammen.

## Architektur

Übernommen vom `d0-smartmeter`-Exporter (gleiche Konventionen):

| Package                                                | Aufgabe                                                                | Herkunft                                 |
|--------------------------------------------------------|------------------------------------------------------------------------|------------------------------------------|
| `pkg/server`                                           | HTTP-Server mit `otelhttp`, `/live` `/ready`, graceful shutdown        | verbatim aus d0                          |
| `pkg/observerbility`                                   | OpenTelemetry-SDK via `autoexport` (Traces)                            | aus d0, Tracer-Name `speedwire-exporter` |
| `pkg/logging`                                          | `slog` (text/json) + `TracingLogHandler` (Trace-Korrelation)           | verbatim aus d0                          |
| `cmd/`                                                 | `root` (viper), `run` (Wiring), `logging`, `version`, `completion`     | aus d0                                   |
| `cmd/readout`                                          | einmaliges Discovern & Werte-Dump (bisheriger PoC `speedwire.Devices`) | Umbau PoC                                |
| `version`                                              | Version/Revision via ldflags                                           | aus d0                                   |
| Makefile, Dockerfile, `.goreleaser.yaml`, CI-Workflows | Build/Release/Test                                                     | auf d0-Stand angleichen                  |

Neue Dependencies: OTel-Stack (`autoexport`, `otelhttp`, `otel/sdk`),
`golang/glog`, `robfig/cron/v3`. Alle Dependencies auf **aktuellstem Stand**
(`go get -u -t` + `go mod tidy`).

### Push-Collector (statt Poll+Cache)

Speedwire ist **push-basiert** (EM sendet ~1×/s). Deshalb ein custom
`prometheus.Collector` nach dem Muster von
`mqtt2prometheus/pkg/metrics/collector.go` (`MemoryCachedCollector`):

- **`pkg/collector`**: implementiert `Describe`/`Collect` + `Observe(serial, snapshots)`.
    - Vorgebaute `*prometheus.Desc` je Metrik, fqName = `<konfigurierter Prefix>` + bare Name aus dem Mapping (Prefix
      pro Gerätetyp aus der Config). Variable Labels `meter`,`phase`[,`direction`].
    - Die `<prefix>_info`-Metrik wird nur registriert/emittiert, wenn `metrics.info`
      aktiv ist.
    - **Expiring go-cache** (Timeout z.B. 30s): Werte verschwinden automatisch, wenn der EM verstummt.
    - `Collect()` emittiert je Cache-Eintrag via `prometheus.MustNewConstMetric`, umschlossen mit
      `prometheus.NewMetricWithTimestamp(observeTime, …)`.
- **`pkg/speedwire`**: `sunny.NewConnection(iface)` + `DiscoverDevices`; pro entdecktem Gerät im Intervall
  `GetValuesCtx` lesen und — je nach
  `dev.IsEnergyMeter()` — `collector.Observe(serial, mapEnergyMeter/mapInverter(values))`
  aufrufen. Geräte ohne Config-Eintrag werden geloggt, nicht exportiert.

### Readout / Discovery

Onboarding-Hilfe, um Serials und verfügbare Werte eines neuen Setups zu sehen:

- **CLI** `speedwire-exporter readout`: discovern, je Gerät alle Werte (`sunny.GetValueInfo`) tabellarisch ausgeben
  (Umbau des bisherigen PoC
  `speedwire.Devices`).
- **HTTP** `/devices` am Exporter-Port: JSON-Dump aller aktuell entdeckten Geräte (serial, address, isEnergyMeter, alle
  Werte mit Beschreibung/Einheit). Unabhängig von der Config — zeigt auch nicht registrierte Geräte.
- **`pkg/mapping`** (o.ä.): **reine Funktionen** je Gerätetyp,
  `mapEnergyMeter(...)` und `mapInverter(map[sunny.ValueID]interface{}) []Snapshot`
  — Signierung (nur EM), Einheiten-Umrechnung (Ws→kWh), Label-Zuordnung. Handhaben `interface{}` (uint64/float64) per
  Type-Switch. Gerätetyp-Auswahl via
  `dev.IsEnergyMeter()`. Die Funktionen liefern **bare** Metriknamen (ohne Prefix) — den Prefix hängt der Collector aus
  der Config an. **Das ist der TDD-Kern.**

`Snapshot` = `{Name string, Type (gauge|counter), Labels map[string]string, Value float64}`
(`Name` ohne Prefix).

### Wiring (`cmd/run.go`)

`viper.Unmarshal(&config)` → `server.NewHttpServer(port)` → Registry mit registriertem Collector,
`promhttp.HandlerFor(...OpenMetrics)` an
`/metrics` → Liveness „letztes Observe < N" → Speedwire-Listener starten →
`httpServer.Run(ctx)` blockt (graceful shutdown via Signal/Context).

## Config

```yaml
exporter: { port: 8080, goMetrics: false }
logging:  { level: info, format: json }
fetchInterval: 5s
interface: net1               # Multicast-Interface (Multus im Cluster)
discovery: { password: "0000" }
metrics:
  energyMeterPrefix: smartmeter      # Default; frei konfigurierbar
  inverterPrefix: sunny_inverter     # Default; frei konfigurierbar
  info: false                        # <prefix>_info (SoftwareVersion/Gerätedaten) an/aus
devices:                      # Serial → Labels; Gerätetyp aus Discovery
  - serial: 1234567890
    labels: { meter: grid }
```

Die Prefixes sind pro **Gerätetyp** konfigurierbar (nicht pro Gerät — mehrere Zähler unterscheiden sich über
Label-Werte, nicht über den Namen). Die in den Mapping-Tabellen genannten Namen sind die Defaults.

## Testing (TDD)

- `pkg/mapping`: Tabellen-Tests gegen Fake-`map[sunny.ValueID]interface{}` für
  `mapEnergyMeter` (Signierung/Einspeisung negativ, Ws→kWh, `total` vs `l1..3`, direction) und `mapInverter` (AC/DC-
  `side`, `interval`, today=gauge, keine Signierung) — inkl. gemischter Typen (uint64/float64) und fehlender Werte.
- `pkg/collector`: `Observe` → `Collect` liefert erwartete Metriken/Labels; Cache-Expiry entfernt verstummte Werte.
- `pkg/speedwire`: dünn; sunny-Zugriff hinter kleinem Interface für Fakes.

## Phasen

- **P0 – Gerüst:** d0-Packages portieren (`server`,`observerbility`,`logging`,
  `cmd/*`,`version`), Build/CI angleichen, `run()` verdrahten,
  `main.go` → `cmd.Execute()`, `readout` (CLI + `/devices`). Ergebnis:
  laufender Exporter mit `/metrics` (leer) + Health + Discovery.
- **P1 – Energy Meter:** `mapEnergyMeter` (TDD) → `pkg/collector` →
  `pkg/speedwire`. Ergebnis: `smartmeter_*{meter="grid"}` mit signierter Leistung + directional Energie-Countern.
- **P2 – Wechselrichter:** `mapInverter` (TDD), Anschluss an denselben Collector. Ergebnis: `sunny_inverter_*` für
  speedwire-fähige WR (STPH-X).
- **P3 – Deployment (im `kampenwand`-Repo):** k8s-Manifeste unter
  `clusters/kampenwand/smarthome/speedwire-exporter/` (Deployment mit Multus-Multicast-Zugang analog evcc, Service,
  Probe, Kustomization, Image `ghcr.io/chr-fritz/speedwire-exporter`). **Entfernen:**
  `mqtt2prometheus/sma-em` + `images/sma-em`. `sbfspot` bleibt.

## Offene Punkte

- Wert-Zeitstempel: sunny liefert keinen sauberen Per-Wert-Timestamp → Observe-Zeit als Metrik-Timestamp verwenden.
- Genaues Multus-/Multicast-Setup wird in P3 (kampenwand) festgelegt.

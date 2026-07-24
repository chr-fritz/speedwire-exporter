package mapping

import (
	"fmt"

	"gitlab.com/bboehmke/sunny"
)

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

func kwhCounters(values map[sunny.ValueID]interface{}, defs []kwhDef) []Snapshot {
	var out []Snapshot
	for _, d := range defs {
		if f, ok := toFloat(values[d.id]); ok {
			out = append(out, Snapshot{Name: d.name, Type: Counter, Labels: d.labels, Value: f})
		}
	}
	return out
}

// MapInverter converts a raw sunny inverter value map into bare metric snapshots.
func MapInverter(values map[sunny.ValueID]interface{}) []Snapshot {
	var out []Snapshot
	out = append(out, gaugesFrom(values, inverterGaugeDefs)...)
	out = append(out, gaugesFrom(values, inverterHybridPower)...)
	out = append(out, kwhCounters(values, inverterEnergyCounters)...)
	out = append(out, gaugesFrom(values, inverterEnergyGauges)...)
	out = append(out, gaugesFrom(values, inverterBattery)...)
	return out
}

var mappedInverterIDs = func() map[sunny.ValueID]struct{} {
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
//
// The label-key set ({software_version, device_name}) is always identical across all
// inverter-info series, even when only one of the two values is reported: an absent
// value is emitted as an empty string rather than an omitted label key. This keeps
// dimensions stable across devices sharing the same metric prefix, since prometheus'
// Gather() (and the default promhttp error handling) fails the entire /metrics response
// if two series of the same metric family carry different label-key sets.
func InverterInfo(values map[sunny.ValueID]interface{}) (Snapshot, bool) {
	softwareVersion, hasSoftwareVersion := values[sunny.SoftwareVersion]
	deviceName, hasDeviceName := values[sunny.DeviceName]
	if !hasSoftwareVersion && !hasDeviceName {
		return Snapshot{}, false
	}

	labels := map[string]string{
		"software_version": "",
		"device_name":      "",
	}
	if hasSoftwareVersion {
		labels["software_version"] = fmt.Sprintf("%v", softwareVersion)
	}
	if hasDeviceName {
		labels["device_name"] = fmt.Sprintf("%v", deviceName)
	}
	return Snapshot{Name: "info", Type: Gauge, Labels: labels, Value: 1}, true
}

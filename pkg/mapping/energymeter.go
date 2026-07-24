package mapping

import (
	"fmt"

	"gitlab.com/bboehmke/sunny"
)

const wsToKWh = 1.0 / 3_600_000.0

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

// MapEnergyMeter converts a raw sunny energy-meter value map into bare metric snapshots.
func MapEnergyMeter(values map[sunny.ValueID]interface{}) []Snapshot {
	var out []Snapshot
	out = append(out, signedPower(values, "active_power", activePower)...)
	out = append(out, signedPower(values, "reactive_power", reactivePower)...)
	out = append(out, signedPower(values, "apparent_power", apparentPower)...)
	out = append(out, directGauges(values)...)
	out = append(out, energyCounters(values)...)
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

func directGauges(values map[sunny.ValueID]interface{}) []Snapshot {
	var out []Snapshot
	for _, d := range directGaugeDefs {
		if f, ok := toFloat(values[d.id]); ok {
			out = append(out, Snapshot{Name: d.name, Type: Gauge, Labels: d.labels, Value: f})
		}
	}
	return out
}

// mappedEnergyMeterIDs is the set of ValueIDs the energy-meter mapping consumes.
var mappedEnergyMeterIDs = func() map[sunny.ValueID]struct{} {
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
	m[sunny.SoftwareVersion] = struct{}{}
	return m
}()

// IsMappedEnergyMeter reports whether the energy-meter mapping consumes the value.
func IsMappedEnergyMeter(id sunny.ValueID) bool {
	_, ok := mappedEnergyMeterIDs[id]
	return ok
}

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

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
		sunny.VoltageS1:         385.4,
		sunny.CurrentL1:         13.9,
		sunny.VoltageL1L2:       399.2,
		sunny.ActivePowerMax:    5000.0,
	}
	snaps := MapInverter(values)

	assert.InDelta(t, 3200.0, findLabeled(t, snaps, "power", map[string]string{"side": "AC", "phase": "total"}).Value, 0.001)
	assert.InDelta(t, 1600.0, findLabeled(t, snaps, "power", map[string]string{"side": "DC", "phase": "1"}).Value, 0.001)
	assert.InDelta(t, 231.0, findLabeled(t, snaps, "voltage", map[string]string{"side": "AC", "phase": "l1"}).Value, 0.001)
	assert.InDelta(t, 5.2, findLabeled(t, snaps, "current", map[string]string{"side": "DC", "phase": "1"}).Value, 0.001)
	assert.Equal(t, Gauge, findLabeled(t, snaps, "power", map[string]string{"side": "AC", "phase": "total"}).Type)

	// DC voltage (string 1)
	assert.InDelta(t, 385.4, findLabeled(t, snaps, "voltage", map[string]string{"side": "DC", "phase": "1"}).Value, 0.001)
	// AC current L1
	assert.InDelta(t, 13.9, findLabeled(t, snaps, "current", map[string]string{"side": "AC", "phase": "l1"}).Value, 0.001)
	// line-to-line voltage L1-L2
	assert.InDelta(t, 399.2, findLabeled(t, snaps, "voltage", map[string]string{"side": "AC", "phase": "l1l2"}).Value, 0.001)
	// frequency (no phase label)
	assert.InDelta(t, 50.0, findLabeled(t, snaps, "frequency", map[string]string{}).Value, 0.001)
	// temperature (no phase label)
	assert.InDelta(t, 42.5, findLabeled(t, snaps, "temperature", map[string]string{}).Value, 0.001)
	// power_max (no phase label)
	assert.InDelta(t, 5000.0, findLabeled(t, snaps, "power_max", map[string]string{}).Value, 0.001)
}

func TestMapInverterEnergyAndHybrid(t *testing.T) {
	values := map[sunny.ValueID]interface{}{
		sunny.ActiveEnergyPlusKWh:      12345.6, // already kWh (no conversion)
		sunny.PvPower:                  4100.0,
		sunny.GridPowerExport:          2500.0,
		sunny.BatteryCharge:            87.0,
		sunny.GridEnergyExportKWh:      987.6,
		sunny.PvEnergyTotalKWh:         54321.0,
		sunny.ActiveEnergyPlusTodayKWh: 12.34,
		sunny.ConsumptionPower:         1800.0,
		sunny.SelfConsumptionPower:     1600.0,
		sunny.BatteryEnergyChargeKWh:   321.9,
		sunny.BatteryTemperature:       28.3,
		sunny.BatteryVoltage:           52.7,
		sunny.BatteryChargeCycles:      412.0,
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

	// pv_energy is a counter sourced from PvEnergyTotalKWh, used as-is (not divided)
	pvEnergy := findLabeled(t, snaps, "pv_energy", map[string]string{})
	assert.Equal(t, Counter, pvEnergy.Type)
	assert.InDelta(t, 54321.0, pvEnergy.Value, 0.01)

	// energy_today is a gauge sourced from ActiveEnergyPlusTodayKWh
	energyToday := findLabeled(t, snaps, "energy_today", map[string]string{})
	assert.Equal(t, Gauge, energyToday.Type)
	assert.InDelta(t, 12.34, energyToday.Value, 0.001)

	assert.InDelta(t, 1800.0, findLabeled(t, snaps, "consumption_power", map[string]string{}).Value, 0.001)
	assert.InDelta(t, 1600.0, findLabeled(t, snaps, "self_consumption_power", map[string]string{}).Value, 0.001)

	// battery_energy{direction=charge} is a counter sourced from BatteryEnergyChargeKWh
	batteryEnergy := findLabeled(t, snaps, "battery_energy", map[string]string{"direction": "charge"})
	assert.Equal(t, Counter, batteryEnergy.Type)
	assert.InDelta(t, 321.9, batteryEnergy.Value, 0.01)

	assert.InDelta(t, 28.3, findLabeled(t, snaps, "battery_temperature", map[string]string{}).Value, 0.001)
	assert.InDelta(t, 52.7, findLabeled(t, snaps, "battery_voltage", map[string]string{}).Value, 0.001)
	assert.InDelta(t, 412.0, findLabeled(t, snaps, "battery_charge_cycles", map[string]string{}).Value, 0.001)
}

func TestIsMappedInverterAndInfo(t *testing.T) {
	assert.True(t, IsMappedInverter(sunny.PowerS1))
	assert.True(t, IsMappedInverter(sunny.PvPower))
	assert.False(t, IsMappedInverter(sunny.ReactiveEnergyPlusL1))

	s, ok := InverterInfo(map[sunny.ValueID]interface{}{sunny.SoftwareVersion: "3.10.24.R"})
	assert.True(t, ok)
	assert.Equal(t, "info", s.Name)
	assert.Equal(t, "3.10.24.R", s.Labels["software_version"])
	// software_version-only input must still carry BOTH label keys, with
	// device_name defaulted to "", so that inverters reporting different
	// subsets of sunny values produce dimension-stable info series.
	assert.Contains(t, s.Labels, "device_name")
	assert.Equal(t, "", s.Labels["device_name"])
	assert.Len(t, s.Labels, 2)

	s2, ok2 := InverterInfo(map[sunny.ValueID]interface{}{sunny.DeviceName: "SB3.6-1AV-41"})
	assert.True(t, ok2)
	assert.Equal(t, "", s2.Labels["software_version"])
	assert.Equal(t, "SB3.6-1AV-41", s2.Labels["device_name"])
	assert.Len(t, s2.Labels, 2)

	_, ok3 := InverterInfo(map[sunny.ValueID]interface{}{})
	assert.False(t, ok3)
}

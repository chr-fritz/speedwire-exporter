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

		// L2 feeds in 350.2 var reactive, total nets -120.5 var (more feed-in than draw)
		sunny.ReactivePowerPlusL2:  0.0,
		sunny.ReactivePowerMinusL2: 350.2,
		sunny.ReactivePowerPlus:    30.0,
		sunny.ReactivePowerMinus:   150.5,

		// L2 feeds in 410.9 VA apparent, total nets -90.1 VA
		sunny.ApparentPowerPlusL2:  0.0,
		sunny.ApparentPowerMinusL2: 410.9,
		sunny.ApparentPowerPlus:    20.0,
		sunny.ApparentPowerMinus:   110.1,
	}
	snaps := MapEnergyMeter(values)

	assert.InDelta(t, 172.6, find(t, snaps, "active_power", "l1").Value, 0.001)
	assert.InDelta(t, -692.9, find(t, snaps, "active_power", "l2").Value, 0.001)
	assert.InDelta(t, 15.4, find(t, snaps, "active_power", "total").Value, 0.001)
	assert.Equal(t, Gauge, find(t, snaps, "active_power", "total").Type)

	// reactive_power: plus - minus, including a negative (feed-in) phase and total
	assert.InDelta(t, -350.2, find(t, snaps, "reactive_power", "l2").Value, 0.001)
	assert.InDelta(t, -120.5, find(t, snaps, "reactive_power", "total").Value, 0.001)

	// apparent_power: plus - minus, including a negative (feed-in) phase and total
	assert.InDelta(t, -410.9, find(t, snaps, "apparent_power", "l2").Value, 0.001)
	assert.InDelta(t, -90.1, find(t, snaps, "apparent_power", "total").Value, 0.001)
}

func TestMapEnergyMeterDirectGauges(t *testing.T) {
	values := map[sunny.ValueID]interface{}{
		sunny.CurrentL1:        1.423,
		sunny.CurrentL2:        2.187,
		sunny.CurrentL3:        0.934,
		sunny.VoltageL1:        231.07,
		sunny.VoltageL2:        229.85,
		sunny.VoltageL3:        232.41,
		sunny.PowerFactor:      0.01,
		sunny.PowerFactorL1:    0.642,
		sunny.PowerFactorL2:    0.718,
		sunny.PowerFactorL3:    0.803,
		sunny.UtilityFrequency: 50.0,
	}
	snaps := MapEnergyMeter(values)

	assert.InDelta(t, 1.423, find(t, snaps, "current", "l1").Value, 0.001)
	assert.InDelta(t, 2.187, find(t, snaps, "current", "l2").Value, 0.001)
	assert.InDelta(t, 0.934, find(t, snaps, "current", "l3").Value, 0.001)
	assert.InDelta(t, 231.07, find(t, snaps, "voltage", "l1").Value, 0.001)
	assert.InDelta(t, 229.85, find(t, snaps, "voltage", "l2").Value, 0.001)
	assert.InDelta(t, 232.41, find(t, snaps, "voltage", "l3").Value, 0.001)
	assert.InDelta(t, 0.642, find(t, snaps, "power_factor", "l1").Value, 0.001)
	assert.InDelta(t, 0.718, find(t, snaps, "power_factor", "l2").Value, 0.001)
	assert.InDelta(t, 0.803, find(t, snaps, "power_factor", "l3").Value, 0.001)
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

// findEnergyCounter returns the energy-counter snapshot matching name/phase/direction, or fails.
func findEnergyCounter(t *testing.T, snaps []Snapshot, name, phase, direction string) Snapshot {
	t.Helper()
	for _, s := range snaps {
		if s.Name == name && s.Labels["phase"] == phase && s.Labels["direction"] == direction {
			return s
		}
	}
	t.Fatalf("snapshot %s{phase=%s,direction=%s} not found", name, phase, direction)
	return Snapshot{}
}

func TestMapEnergyMeterEnergyCountersKWh(t *testing.T) {
	values := map[sunny.ValueID]interface{}{
		// 24 692.866 kWh consumed on total => 24692.866 * 3.6e6 Ws
		sunny.ActiveEnergyPlus:  uint64(24692.866 * 3_600_000),
		sunny.ActiveEnergyMinus: uint64(17724.3048 * 3_600_000),

		// L1 consumption/delivery
		sunny.ActiveEnergyPlusL1:  uint64(8123.456 * 3_600_000),
		sunny.ActiveEnergyMinusL1: uint64(5321.789 * 3_600_000),

		// reactive_energy_total on total
		sunny.ReactiveEnergyPlus:  uint64(1234.5 * 3_600_000),
		sunny.ReactiveEnergyMinus: uint64(987.6 * 3_600_000),

		// apparent_energy_total on total
		sunny.ApparentEnergyPlus:  uint64(2345.6 * 3_600_000),
		sunny.ApparentEnergyMinus: uint64(1098.7 * 3_600_000),
	}
	snaps := MapEnergyMeter(values)

	cons := findEnergyCounter(t, snaps, "energy_total", "total", "consumption")
	assert.Equal(t, Counter, cons.Type)
	assert.InDelta(t, 24692.866, cons.Value, 0.01)

	del := findEnergyCounter(t, snaps, "energy_total", "total", "delivery")
	assert.InDelta(t, 17724.3048, del.Value, 0.01)

	// per-phase (l1) consumption/delivery, confirming the Ws -> kWh conversion
	consL1 := findEnergyCounter(t, snaps, "energy_total", "l1", "consumption")
	assert.Equal(t, Counter, consL1.Type)
	assert.InDelta(t, 8123.456, consL1.Value, 0.01)

	delL1 := findEnergyCounter(t, snaps, "energy_total", "l1", "delivery")
	assert.InDelta(t, 5321.789, delL1.Value, 0.01)

	// reactive_energy_total
	reactiveCons := findEnergyCounter(t, snaps, "reactive_energy_total", "total", "consumption")
	assert.Equal(t, Counter, reactiveCons.Type)
	assert.InDelta(t, 1234.5, reactiveCons.Value, 0.01)

	reactiveDel := findEnergyCounter(t, snaps, "reactive_energy_total", "total", "delivery")
	assert.InDelta(t, 987.6, reactiveDel.Value, 0.01)

	// apparent_energy_total
	apparentCons := findEnergyCounter(t, snaps, "apparent_energy_total", "total", "consumption")
	assert.Equal(t, Counter, apparentCons.Type)
	assert.InDelta(t, 2345.6, apparentCons.Value, 0.01)

	apparentDel := findEnergyCounter(t, snaps, "apparent_energy_total", "total", "delivery")
	assert.InDelta(t, 1098.7, apparentDel.Value, 0.01)
}

func TestIsMappedEnergyMeter(t *testing.T) {
	assert.True(t, IsMappedEnergyMeter(sunny.ActivePowerPlus))
	assert.True(t, IsMappedEnergyMeter(sunny.UtilityFrequency))
	// PowerS1 is a DC-string value an energy meter never reports and the mapping does not consume.
	assert.False(t, IsMappedEnergyMeter(sunny.PowerS1))
}

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

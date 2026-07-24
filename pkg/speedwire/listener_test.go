// Copyright © 2026 Christian Fritz <mail@chr-fritz.de>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package speedwire

import (
	"testing"

	"github.com/chr-fritz/speedwire-exporter/pkg/config"
	"gitlab.com/bboehmke/sunny"
)

func TestDeviceObservations_EnergyMeter_NoInfo(t *testing.T) {
	values := map[sunny.ValueID]interface{}{
		sunny.ActivePowerPlus:  float64(100),
		sunny.ActivePowerMinus: float64(0),
	}
	m := config.MetricsConfig{EnergyMeterPrefix: "smartmeter", InverterPrefix: "sunny_inverter", Info: false}

	obs := deviceObservations(true, values, m)

	if len(obs) != 1 {
		t.Fatalf("expected exactly one observation, got %d: %+v", len(obs), obs)
	}
	if obs[0].prefix != m.EnergyMeterPrefix {
		t.Errorf("prefix = %q, want %q", obs[0].prefix, m.EnergyMeterPrefix)
	}
	if len(obs[0].snaps) == 0 {
		t.Fatalf("expected non-empty snapshots")
	}
	found := false
	for _, s := range obs[0].snaps {
		if s.Name == "active_power" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected an active_power snapshot, got %+v", obs[0].snaps)
	}
}

func TestDeviceObservations_EnergyMeter_WithInfo(t *testing.T) {
	values := map[sunny.ValueID]interface{}{
		sunny.ActivePowerPlus:  float64(100),
		sunny.ActivePowerMinus: float64(0),
		sunny.SoftwareVersion:  "1.2.3",
	}
	m := config.MetricsConfig{EnergyMeterPrefix: "smartmeter", InverterPrefix: "sunny_inverter", Info: true}

	obs := deviceObservations(true, values, m)

	if len(obs) != 2 {
		t.Fatalf("expected exactly two observations, got %d: %+v", len(obs), obs)
	}
	infoObs := obs[1]
	if infoObs.prefix != m.EnergyMeterPrefix {
		t.Errorf("prefix = %q, want %q", infoObs.prefix, m.EnergyMeterPrefix)
	}
	if len(infoObs.snaps) != 1 {
		t.Fatalf("expected exactly one info snapshot, got %d: %+v", len(infoObs.snaps), infoObs.snaps)
	}
	if infoObs.snaps[0].Name != "info" {
		t.Errorf("snapshot name = %q, want %q", infoObs.snaps[0].Name, "info")
	}
}

func TestDeviceObservations_Inverter_NoInfo(t *testing.T) {
	values := map[sunny.ValueID]interface{}{
		sunny.ActivePowerPlus: float64(500),
		sunny.PowerS1:         float64(250),
	}
	m := config.MetricsConfig{EnergyMeterPrefix: "smartmeter", InverterPrefix: "sunny_inverter", Info: false}

	obs := deviceObservations(false, values, m)

	if len(obs) != 1 {
		t.Fatalf("expected exactly one observation, got %d: %+v", len(obs), obs)
	}
	if obs[0].prefix != m.InverterPrefix {
		t.Errorf("prefix = %q, want %q", obs[0].prefix, m.InverterPrefix)
	}
	found := false
	for _, s := range obs[0].snaps {
		if s.Name == "power" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a power snapshot, got %+v", obs[0].snaps)
	}
}

func TestMappedPredicate(t *testing.T) {
	emPredicate := mappedPredicate(true)
	if !emPredicate(sunny.ActivePowerPlus) {
		t.Errorf("expected energy-meter predicate to recognize ActivePowerPlus")
	}
	if emPredicate(sunny.PowerS1) {
		t.Errorf("expected energy-meter predicate to not recognize PowerS1")
	}

	inverterPredicate := mappedPredicate(false)
	if !inverterPredicate(sunny.PowerS1) {
		t.Errorf("expected inverter predicate to recognize PowerS1")
	}
}

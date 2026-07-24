// Copyright © 2020-2022 Christian Fritz <mail@chr-fritz.de>
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

package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/chr-fritz/speedwire-exporter/pkg/speedwire"
	"gitlab.com/bboehmke/sunny"
)

// DiscoverFunc discovers Speedwire devices and returns them along with their current values.
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

// newDevicesHandler returns an http.HandlerFunc that discovers all Speedwire devices reachable via the given
// DiscoverFunc and renders them as JSON, independent of any configured device filtering.
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

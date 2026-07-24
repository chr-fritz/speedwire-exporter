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
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var out []map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out, 1)
	assert.EqualValues(t, 42, out[0]["serial"])
	assert.Equal(t, true, out[0]["isEnergyMeter"])

	values, ok := out[0]["values"].([]interface{})
	require.True(t, ok)
	require.Len(t, values, 1)

	value, ok := values[0].(map[string]interface{})
	require.True(t, ok)

	info := sunny.GetValueInfo(sunny.ActivePowerPlus)
	assert.EqualValues(t, int(sunny.ActivePowerPlus), value["id"])
	assert.Equal(t, info.Description, value["description"])
	assert.Equal(t, "W", info.Unit)
	assert.Equal(t, info.Unit, value["unit"])
	assert.EqualValues(t, 15.4, value["value"])
}

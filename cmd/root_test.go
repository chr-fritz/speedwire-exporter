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
	"testing"

	"github.com/chr-fritz/speedwire-exporter/pkg/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigureEnv_DiscoveryPasswordFromEnv is a regression test for the
// SPEEDWIRE_DISCOVERY_PASSWORD environment variable override: without the
// env binding, viper.Unmarshal would silently leave Discovery.Password empty
// when no config file sets it, even though the env var is present.
func TestConfigureEnv_DiscoveryPasswordFromEnv(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	t.Setenv("SPEEDWIRE_DISCOVERY_PASSWORD", "s3cret")

	configureEnv()

	var cfg config.Config
	require.NoError(t, viper.Unmarshal(&cfg))
	assert.Equal(t, "s3cret", cfg.Discovery.Password)
}

func TestConfigureEnv_NoEnvLeavesDefaultEmpty(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	configureEnv()

	var cfg config.Config
	require.NoError(t, viper.Unmarshal(&cfg))
	assert.Empty(t, cfg.Discovery.Password)
}

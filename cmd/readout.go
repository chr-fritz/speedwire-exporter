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
	"fmt"
	"log/slog"
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
	if err := viper.Unmarshal(&cfg); err != nil {
		slog.With("err", err).Warn("Can not read configuration")
	}

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

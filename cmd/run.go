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
	"log/slog"

	"github.com/chr-fritz/speedwire-exporter/pkg/collector"
	"github.com/chr-fritz/speedwire-exporter/pkg/config"
	"github.com/chr-fritz/speedwire-exporter/pkg/server"
	"github.com/chr-fritz/speedwire-exporter/pkg/speedwire"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const RunPortParm = "exporter.port"
const WithGoMetricsParm = "exporter.goMetrics"

func NewRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the exporter",
		Args:  cobra.NoArgs,
		RunE:  run,
	}
	cmd.Flags().Uint16P("port", "p", 8080, "The port where metrics are exported.")
	cmd.Flags().BoolP("withGoMetrics", "g", false, "Also export Go runtime and process metrics.")
	_ = viper.BindPFlag(RunPortParm, cmd.Flags().Lookup("port"))
	_ = viper.BindPFlag(WithGoMetricsParm, cmd.Flags().Lookup("withGoMetrics"))
	return cmd
}

func run(cmd *cobra.Command, _ []string) error {
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		slog.With("err", err).Error("Can not read configuration")
		return err
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	httpServer, err := server.NewHttpServer(uint16(viper.GetUint(RunPortParm)))
	if err != nil {
		slog.With("err", err).Error("Can not start http server")
		return err
	}

	registry := newRegistry(viper.GetBool(WithGoMetricsParm))
	httpServer.AddHandler("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
		ErrorHandling:     promhttp.ContinueOnError,
	}))
	httpServer.AddHandleFunc("/devices", newDevicesHandler(func(ctx context.Context) ([]speedwire.DiscoveredDevice, error) {
		return speedwire.Discover(ctx, cfg.Interface, cfg.Discovery.Password)
	}))
	httpServer.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(100))

	col := collector.NewCollector()
	if err = registry.Register(col); err != nil {
		slog.With("err", err).Error("can not register collector")
		return err
	}
	listener := speedwire.NewListener(&cfg, col)
	go listener.Run(ctx)

	return httpServer.Run(ctx)
}

func newRegistry(withGo bool) *prometheus.Registry {
	registry := prometheus.NewRegistry()
	if withGo {
		registry.MustRegister(
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		)
	}
	return registry
}

func init() {
	rootCmd.AddCommand(NewRunCommand())
}

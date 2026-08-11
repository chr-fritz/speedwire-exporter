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
	"log/slog"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// freshnessCollector exposes, per configured device, when it was last read successfully.
//
// The device metrics themselves cannot answer "is this exporter still working?": they expire
// from the collector cache 30s after readings stop, so a broken exporter and a healthy one
// that happens to have nothing to report both serve an empty /metrics. This gauge keeps
// reporting, which makes staleness alertable per device - a single global timestamp would
// stay fresh while one of several configured devices is dead.
//
// A device that has never answered reports 0 rather than being omitted, so that a device
// which never came up at all is visible instead of silently absent.
type freshnessCollector struct {
	desc      *prometheus.Desc
	lastReads func() map[uint32]time.Time
}

func newFreshnessCollector(lastReads func() map[uint32]time.Time) prometheus.Collector {
	return freshnessCollector{
		desc: prometheus.NewDesc(
			"speedwire_last_successful_read_timestamp_seconds",
			"Unix timestamp of the last successful read of this device, 0 if it has never been read.",
			[]string{"serial"}, nil,
		),
		lastReads: lastReads,
	}
}

func (c freshnessCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c freshnessCollector) Collect(ch chan<- prometheus.Metric) {
	for serial, t := range c.lastReads() {
		var seconds float64
		if !t.IsZero() {
			seconds = float64(t.UnixNano()) / float64(time.Second)
		}

		serialStr := strconv.FormatUint(uint64(serial), 10)
		m, err := prometheus.NewConstMetric(c.desc, prometheus.GaugeValue, seconds, serialStr)
		if err != nil {
			// Deliberately not MustNewConstMetric: a panic here would happen on the scrape
			// goroutine and take the whole exporter down over a metric that only reports health.
			slog.With("serial", serialStr, "err", err).Warn("can not build freshness metric")
			continue
		}
		ch <- m
	}
}

package collector

import (
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/chr-fritz/speedwire-exporter/pkg/mapping"
	"github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"
)

const defaultTimeout = 30 * time.Second

type Collector struct {
	cache *cache.Cache
	now   func() time.Time
}

func NewCollector() *Collector {
	return &Collector{
		cache: cache.New(defaultTimeout, defaultTimeout*10),
		now:   time.Now,
	}
}

// Describe leaves the collector unchecked (dynamic label sets).
func (c *Collector) Describe(chan<- *prometheus.Desc) {}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	for _, item := range c.cache.Items() {
		if m, ok := item.Object.(prometheus.Metric); ok {
			ch <- m
		}
	}
}

func (c *Collector) Observe(prefix, serial string, snaps []mapping.Snapshot, constLabels map[string]string) {
	for _, s := range snaps {
		labels := map[string]string{}
		for k, v := range constLabels {
			labels[k] = v
		}
		for k, v := range s.Labels {
			labels[k] = v
		}

		names, values := sortedLabels(labels)
		fqName := prefix + "_" + s.Name
		desc := prometheus.NewDesc(fqName, "", names, nil)

		valueType := prometheus.GaugeValue
		if s.Type == mapping.Counter {
			valueType = prometheus.CounterValue
		}

		metric, err := prometheus.NewConstMetric(desc, valueType, s.Value, values...)
		if err != nil {
			slog.With("metric", fqName, "err", err).Warn("can not build metric")
			continue
		}
		metric = prometheus.NewMetricWithTimestamp(c.now(), metric)
		c.cache.Set(serial+"|"+fqName+"|"+strings.Join(values, ","), metric, cache.DefaultExpiration)
	}
}

func sortedLabels(labels map[string]string) (names, values []string) {
	names = make([]string, 0, len(labels))
	for k := range labels {
		names = append(names, k)
	}
	sort.Strings(names)
	values = make([]string, 0, len(labels))
	for _, k := range names {
		values = append(values, labels[k])
	}
	return names, values
}

package services

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	metric = &Metric{
		impfail: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "tagger_import_failures",
				Help: "The total number of tag import failures",
			},
		),
		impsucc: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "tagger_import_success",
				Help: "The total number of tag import successes",
			},
		),
		impdura: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "tagger_import_duration",
				Help:    "Duration of a tag import",
				Buckets: []float64{1, 2, 3, 5, 8, 10, 30, 60, 90, 120, 150, 180},
			},
			[]string{"cached"},
		),
		actwrkr: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "tag_current_active_workers",
			Help: "Current number of active tag processing workers",
		}),
	}
	prometheus.MustRegister(
		metric.impfail,
		metric.impsucc,
		metric.impdura,
		metric.actwrkr,
	)
}

// metric holds a singleton of a Metric instance.
var metric *Metric

// Metric is a struc containing all metrics within the system.
type Metric struct {
	impfail prometheus.Counter
	impsucc prometheus.Counter
	impdura *prometheus.HistogramVec
	actwrkr prometheus.Gauge
}

// NewMetrics returns a singleton instance of Metric struct.
func NewMetrics() *Metric {
	return metric
}

// ReportImportFailure increases the number of tag import failures.
func (m *Metric) ReportImportFailure() {
	m.impfail.Inc()
}

// ReportImportSuccess increases the number of tag import failures.
func (m *Metric) ReportImportSuccess() {
	m.impsucc.Inc()
}

// ReportImportDuration registers a new import duration on a prometheus metric.
func (m *Metric) ReportImportDuration(dur time.Duration, cached bool) {
	cachedstr := "no"
	if cached {
		cachedstr = "yes"
	}
	m.impdura.WithLabelValues(cachedstr).Observe(dur.Seconds())
}

// ReportWorker registers work activivy state. If active is true it means a running
// worker is running, false means a worker finished its job.
func (m *Metric) ReportWorker(active bool) {
	if active {
		m.actwrkr.Inc()
		return
	}
	m.actwrkr.Dec()
}

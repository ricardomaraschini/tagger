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
			[]string{"cache", "nocache"},
		),
	}
	prometheus.MustRegister(metric.impfail)
}

// metric holds a singleton of a Metric instance.
var metric *Metric

// Metric is a struc containing all metrics within the system.
type Metric struct {
	impfail prometheus.Counter
	impsucc prometheus.Counter
	impdura *prometheus.HistogramVec
}

// GetMetrics returns a singleton instance of Metric struct.
func GetMetrics() *Metric {
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

// ReportImportTime registers a new import duration on a prometheus metric.
func (m *Metric) ReportImportTime(dur time.Duration, cached bool) {
	lbl := "nocache"
	if cached {
		lbl = "cache"
	}
	m.impdura.WithLabelValues(lbl).Observe(dur.Seconds())
}

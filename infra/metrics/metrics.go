package metrics

import "github.com/prometheus/client_golang/prometheus"

// Bellow follow a list of all registered metrics we have in our system. To add a new one
// remember to add it to this list and also to properly register it on init() func.
var (
	ImportFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tagger_import_failures",
			Help: "The total number of image import failures",
		},
	)
	ImportSuccesses = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tagger_import_success",
			Help: "The total number of image import successes",
		},
	)
	PushSuccesses = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tagger_image_pushes_success",
			Help: "The total number image pushes",
		},
	)
	PushFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tagger_image_pushes_failures",
			Help: "The total number failed image pushes",
		},
	)
	PushLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "tagger_push_latency",
			Help:    "Time spent processsing image pulls",
			Buckets: []float64{5, 10, 15, 20, 30, 45, 60, 90, 120, 150, 180, 300, 600},
		},
	)
	PullSuccesses = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tagger_image_pulls_success",
			Help: "The total number of image pulls",
		},
	)
	PullFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tagger_image_pulls_failures",
			Help: "The total number of failed image pulls",
		},
	)
	PullLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "tagger_pull_latency",
			Help:    "Time spent processsing image pulls",
			Buckets: []float64{5, 10, 15, 20, 30, 45, 60, 90, 120, 150, 180, 300, 600},
		},
	)
	ActiveWorkers = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "tagger_active_import_workers",
			Help: "Current number of running image imports workers",
		},
	)
	MirrorLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "tagger_mirror_latency",
			Help:    "Time spent mirroring images",
			Buckets: []float64{5, 10, 15, 20, 30, 45, 60, 90, 120, 150, 180, 300, 600},
		},
	)
)

func init() {
	prometheus.MustRegister(
		ImportFailures,
		ImportSuccesses,
		PushSuccesses,
		PushFailures,
		PushLatency,
		PullSuccesses,
		PullFailures,
		PullLatency,
		ActiveWorkers,
		MirrorLatency,
	)
}

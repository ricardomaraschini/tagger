package metrics

import "github.com/prometheus/client_golang/prometheus"

// Bellow follow a list of all registered metrics we have in our system. To add a new one
// remember to add it to this list and also to properly register it on init() func.
var (
	ImportFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tagger_import_failures",
			Help: "The total number of tag import failures",
		},
	)
	ImportSuccesses = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tagger_import_success",
			Help: "The total number of tag import successes",
		},
	)
	ImagePushes = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tagger_image_pushes",
			Help: "The total number image pushes",
		},
	)
	ImagePulls = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tagger_image_pulls",
			Help: "The total number of image pulls",
		},
	)
	ActiveWorkers = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "tagger_active_workers",
			Help: "Current number of running image imports workers",
		},
	)
	MirrorLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "tagger_mirror_latency",
			Help:    "Time spent mirroring images",
			Buckets: []float64{5, 10, 15, 20, 30, 45, 60, 90, 120, 150, 180, 300},
		},
	)
)

func init() {
	prometheus.MustRegister(
		ImportFailures,
		ImportSuccesses,
		ImagePushes,
		ImagePulls,
		ActiveWorkers,
		MirrorLatency,
	)
}

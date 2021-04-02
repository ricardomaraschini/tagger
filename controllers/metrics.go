package controllers

import (
	"context"
	"net/http"
	"time"

	"k8s.io/klog/v2"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metric is our controller for metric requests. Spawns an http metric and
// exposes all metrics registered on prometheus.
type Metric struct {
	bind string
}

// NewMetric returns a new metric controller.
func NewMetric() *Metric {
	return &Metric{
		bind: ":8090",
	}
}

// Name returns a name identifier for this controller.
func (m *Metric) Name() string {
	return "metrics http server"
}

// RequiresLeaderElection returns if this controller requires or not a
// leader lease to run.
func (m *Metric) RequiresLeaderElection() bool {
	return false
}

// Start puts the metrics http server online.
func (m *Metric) Start(ctx context.Context) error {
	server := &http.Server{
		Addr:    m.bind,
		Handler: promhttp.Handler(),
	}

	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			klog.Errorf("error shutting down https server: %s", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil {
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
	return nil
}

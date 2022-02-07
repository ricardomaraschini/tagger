// Copyright 2020 The Tagger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"net/http"
	"time"

	"k8s.io/klog/v2"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metric is our controller for metric requests. Spawns an http metric and exposes all metrics
// registered on prometheus (see infra/metrics package to see what are we monitoring).
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

// RequiresLeaderElection returns if this controller requires or not a leader lease to run.
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

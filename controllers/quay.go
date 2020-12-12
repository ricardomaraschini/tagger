package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"k8s.io/klog/v2"
)

// TagGenerationUpdater exists to make tests easier. You may be wondering where
// this is implemented. Please see Tag struct in services/tag.go for a concrete
// implementation.
type TagGenerationUpdater interface {
	NewGenerationForImageRef(context.Context, string) error
}

// QuayRequestPayload holds the information sent by remote quay.io servers
// when a new push has happened to one of images.
type QuayRequestPayload struct {
	Name        string   `json:"name"`
	Repository  string   `json:"repository"`
	Namespace   string   `json:"namespace"`
	DockerURL   string   `json:"docker_url"`
	HomePage    string   `json:"homepage"`
	UpdatedTags []string `json:"updated_tags"`
}

// QuayWebHook handles quay.io requests.
type QuayWebHook struct {
	bind   string
	tagsvc TagGenerationUpdater
}

// NewQuayWebHook returns a web hook handler for quay webhooks.
func NewQuayWebHook(tagsvc TagGenerationUpdater) *QuayWebHook {
	return &QuayWebHook{
		bind:   ":8081",
		tagsvc: tagsvc,
	}
}

// Name returns a name identifier for this controller.
func (q *QuayWebHook) Name() string {
	return "quay webhook"
}

// ServeHTTP handles requests coming in from quay.io.
func (q *QuayWebHook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var payload QuayRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		klog.Errorf("error unmarshaling quay request payload: %s", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	klog.Infof("received update for image: %s", payload.DockerURL)
	for _, tag := range payload.UpdatedTags {
		imgpath := fmt.Sprintf("%s:%s", payload.DockerURL, tag)
		if err := q.tagsvc.NewGenerationForImageRef(r.Context(), imgpath); err != nil {
			klog.Errorf("error updating tag %s by reference: %s", imgpath, err)
			http.Error(
				w,
				http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError,
			)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
}

// Start puts the http server online.
func (q *QuayWebHook) Start(ctx context.Context) error {
	server := &http.Server{
		Addr:    q.bind,
		Handler: q,
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

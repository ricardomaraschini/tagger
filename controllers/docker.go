package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"k8s.io/klog/v2"
)

// DockerRequestPayload is sent by docker hub whenever a new push happen to a
// repository.
type DockerRequestPayload struct {
	PushData struct {
		Tag string `json:"tag"`
	} `json:"push_data"`
	Repository struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"repository"`
}

// valid validates the docker payload.
func (d *DockerRequestPayload) valid() bool {
	if d.PushData.Tag == "" {
		return false
	}
	if d.Repository.Name == "" {
		return false
	}
	if d.Repository.Namespace == "" {
		return false
	}
	return true
}

// DockerWebHook handles docker.io requests.
type DockerWebHook struct {
	bind   string
	tagsvc TagGenerationUpdater
}

// NewDockerWebHook returns a web hook handler for docker.io webhooks.
func NewDockerWebHook(tagsvc TagGenerationUpdater) *DockerWebHook {
	return &DockerWebHook{
		bind:   ":8082",
		tagsvc: tagsvc,
	}
}

// Name returns a name identifier for this controller.
func (d *DockerWebHook) Name() string {
	return "docker hub webhook"
}

// RequiresLeaderElection returns if this controller requires or not a
// leader lease to run.
func (d *DockerWebHook) RequiresLeaderElection() bool {
	return false
}

// ServeHTTP handles requests coming in from docker.io.
func (d *DockerWebHook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var payload DockerRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		klog.Errorf("error unmarshaling docker request payload: %s", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if !payload.valid() {
		klog.Errorf("invalid docker payload: %+v", payload)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	imgpath := fmt.Sprintf(
		"docker.io/%s/%s:%s",
		payload.Repository.Namespace,
		payload.Repository.Name,
		payload.PushData.Tag,
	)
	klog.Infof("received update for image: %s", imgpath)
	if err := d.tagsvc.NewGenerationForImageRef(r.Context(), imgpath); err != nil {
		klog.Errorf("error updating tag %s by reference: %s", imgpath, err)
		http.Error(
			w,
			http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError,
		)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(http.StatusText(http.StatusOK)))
}

// Start puts the http server online.
func (d *DockerWebHook) Start(ctx context.Context) error {
	server := &http.Server{
		Addr:    d.bind,
		Handler: d,
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

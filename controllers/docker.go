package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"k8s.io/klog/v2"

	"github.com/ricardomaraschini/tagger/services"
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

// DockerWebHook handles docker.io requests.
type DockerWebHook struct {
	bind   string
	tagsvc *services.Tag
}

// NewDockerWebHook returns a web hook handler for docker.io webhooks.
func NewDockerWebHook(tagsvc *services.Tag) *DockerWebHook {
	return &DockerWebHook{
		bind:   ":8082",
		tagsvc: tagsvc,
	}
}

// Name returns a name identifier for this controller.
func (d *DockerWebHook) Name() string {
	return "docker hub webhook"
}

// ServeHTTP handles requests coming in from docker.io.
func (d *DockerWebHook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var payload DockerRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		klog.Errorf("error unmarshaling docker request payload: %s", err)
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
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
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

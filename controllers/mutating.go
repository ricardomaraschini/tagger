package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	admnv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	imgtagv1 "github.com/ricardomaraschini/tagger/infra/tags/v1"
)

// MutatingWebHook handles Mutation requests from kubernetes api.
type MutatingWebHook struct {
	key     string
	cert    string
	bind    string
	decoder runtime.Decoder
}

// NewMutatingWebHook returns a web hook handler for kubernetes api mutation
// requests.
func NewMutatingWebHook() *MutatingWebHook {
	runtimeScheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(runtimeScheme)
	return &MutatingWebHook{
		key:     "assets/server.key",
		cert:    "assets/server.crt",
		bind:    ":8080",
		decoder: codecs.UniversalDeserializer(),
	}
}

// Name returns a name identifier for this controller.
func (m *MutatingWebHook) Name() string {
	return "mutating webhook"
}

// RequiresLeaderElection returns if this controller requires or not a
// leader lease to run.
func (m *MutatingWebHook) RequiresLeaderElection() bool {
	return false
}

// responseError writes on the response an AdmissionReview with response status
// set to an error. If AdmissionReview contains an UID that is inserted into
// the reply.
func (m *MutatingWebHook) responseError(
	w http.ResponseWriter, req *admnv1.AdmissionReview, err error,
) {
	var ruid types.UID
	if req.Request != nil {
		ruid = req.Request.UID
	}

	reviewResp := &admnv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: &admnv1.AdmissionResponse{
			UID: ruid,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		},
	}
	resp, err := json.Marshal(reviewResp)
	if err != nil {
		errstr := fmt.Sprintf("error encoding response: %v", err)
		http.Error(w, errstr, http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(resp)
}

// responseAuthorized informs kubernetes the object creation is authorized
// without modifications (patch to be applied).
func (m *MutatingWebHook) responseAuthorized(w http.ResponseWriter, req *admnv1.AdmissionReview) {
	var ruid types.UID
	if req.Request != nil {
		ruid = req.Request.UID
	}

	reviewResp := &admnv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: &admnv1.AdmissionResponse{
			Allowed: true,
			UID:     ruid,
		},
	}
	resp, err := json.Marshal(reviewResp)
	if err != nil {
		errstr := fmt.Sprintf("error encoding response: %v", err)
		http.Error(w, errstr, http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(resp)
}

// tag validates a tag during update.
func (m *MutatingWebHook) tag(w http.ResponseWriter, r *http.Request) {
	reviewReq := &admnv1.AdmissionReview{}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		klog.Errorf("error reading body: %s", err)
		m.responseError(w, reviewReq, err)
		return
	}

	if _, _, err := m.decoder.Decode(body, nil, reviewReq); err != nil {
		klog.Errorf("cant decoding body: %s", err)
		m.responseError(w, reviewReq, err)
		return
	}

	objkind := reviewReq.Request.Kind.Kind
	if objkind != "Tag" {
		klog.Errorf("received event for %s, authorizing", objkind)
		m.responseAuthorized(w, reviewReq)
		return
	}

	var tag imgtagv1.Tag
	if err := json.Unmarshal(reviewReq.Request.Object.Raw, &tag); err != nil {
		klog.Errorf("unable to decode tag: %s", err)
		m.responseError(w, reviewReq, err)
		return
	}

	if err := tag.ValidateTagGeneration(); err != nil {
		m.responseError(w, reviewReq, err)
		return
	}

	reviewResp := &admnv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: &admnv1.AdmissionResponse{
			Allowed: true,
			UID:     reviewReq.Request.UID,
		},
	}

	resp, err := json.Marshal(reviewResp)
	if err != nil {
		errstr := fmt.Sprintf("error encoding response: %v", err)
		http.Error(w, errstr, http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(resp)
}

// Start puts the http server online.
func (m *MutatingWebHook) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/tag", m.tag)
	server := &http.Server{
		Addr:    m.bind,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			klog.Errorf("error shutting down https server: %s", err)
		}
	}()

	if err := server.ListenAndServeTLS(m.cert, m.key); err != nil {
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
	return nil
}

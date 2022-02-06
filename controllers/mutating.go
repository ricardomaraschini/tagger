// Copyright 2020 The Imageger Authors.
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

	imgv1b1 "github.com/ricardomaraschini/tagger/infra/images/v1beta1"
)

// ImageImportValidator is implemented in services/imageimport.go. This abstraction
// exists to make tests easier. It is anything capable of checking if a given
// ImageImport is valid (contain all needed fields and refers to a valid Image).
type ImageImportValidator interface {
	Validate(context.Context, *imgv1b1.ImageImport) error
}

// ImageValidator is implemented in services/image.go. Validates that provided Image
// contain all mandatory fields.
type ImageValidator interface {
	Validate(context.Context, *imgv1b1.Image) error
}

// MutatingWebHook handles Mutation requests from kubernetes api.
// I.E. validate Image objects.
type MutatingWebHook struct {
	key     string
	cert    string
	bind    string
	tival   ImageImportValidator
	imgval  ImageValidator
	decoder runtime.Decoder
}

// NewMutatingWebHook returns a web hook handler for kubernetes api
// mutation requests. This webhook validate Image objects when user saves
// them.
func NewMutatingWebHook(tival ImageImportValidator, imgval ImageValidator) *MutatingWebHook {
	runtimeScheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(runtimeScheme)

	olmCertDir := "/tmp/k8s-webhook-server/serving-certs"
	return &MutatingWebHook{
		key:     fmt.Sprintf("%s/tls.key", olmCertDir),
		cert:    fmt.Sprintf("%s/tls.crt", olmCertDir),
		bind:    ":8080",
		tival:   tival,
		imgval:  imgval,
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

// responseError writes in the provided ResponseWriter an AdmissionReview
// with response status set to an error. If AdmissionReview contains an UID
// that is inserted into the reply.
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
// without modifications.
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

// imageimport is our http handler for ImageImport objects validation.
func (m *MutatingWebHook) imageimport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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
	if objkind != "ImageImport" {
		klog.Errorf("received event for %s, authorizing", objkind)
		m.responseAuthorized(w, reviewReq)
		return
	}

	timp := &imgv1b1.ImageImport{}
	if err := json.Unmarshal(reviewReq.Request.Object.Raw, timp); err != nil {
		klog.Errorf("unable to decode image: %s", err)
		m.responseError(w, reviewReq, err)
		return
	}

	if err := m.tival.Validate(ctx, timp); err != nil {
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

// image is our http handler for image validation.
func (m *MutatingWebHook) image(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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
	if objkind != "Image" {
		klog.Errorf("received event for %s, authorizing", objkind)
		m.responseAuthorized(w, reviewReq)
		return
	}

	img := &imgv1b1.Image{}
	if err := json.Unmarshal(reviewReq.Request.Object.Raw, img); err != nil {
		klog.Errorf("unable to decode image: %s", err)
		m.responseError(w, reviewReq, err)
		return
	}

	if err := m.imgval.Validate(ctx, img); err != nil {
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
	mux.HandleFunc("/image", m.image)
	mux.HandleFunc("/imageimport", m.imageimport)
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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	admnv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	"github.com/mattbaird/jsonpatch"

	imgtagv1 "github.com/ricardomaraschini/tagger/imagetags/v1"
)

// PodPatcher creates a patch for a pod resource, possibly overwritting
// tag references by their concrete location. You might want to look at
// the concrete implementation of this at services/tag.go.
type PodPatcher interface {
	PatchForPod(pod corev1.Pod) ([]jsonpatch.JsonPatchOperation, error)
}

// MutatingWebHook handles Mutation requests from kubernetes api.
type MutatingWebHook struct {
	key     string
	cert    string
	bind    string
	tagsvc  PodPatcher
	decoder runtime.Decoder
}

// NewMutatingWebHook returns a web hook handler for kubernetes api mutation
// requests.
func NewMutatingWebHook(tagsvc PodPatcher) *MutatingWebHook {
	runtimeScheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(runtimeScheme)
	return &MutatingWebHook{
		key:     "assets/server.key",
		cert:    "assets/server.crt",
		bind:    ":8080",
		decoder: codecs.UniversalDeserializer(),
		tagsvc:  tagsvc,
	}
}

// Name returns a name identifier for this controller.
func (m *MutatingWebHook) Name() string {
	return "mutating webhook"
}

// responseError writes on the response an AdmissionReview with response status
// set to an error. If AdmissionReview contains an UID that is inserted into
// the reply.
func (m *MutatingWebHook) responseError(w http.ResponseWriter, req *admnv1.AdmissionReview, err error) {
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

// pod handles mutation requests made by kubernetes api with regards to pods.
func (m *MutatingWebHook) pod(w http.ResponseWriter, r *http.Request) {
	reviewReq := &admnv1.AdmissionReview{}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		klog.Errorf("error reading body: %s", err)
		m.responseError(w, reviewReq, err)
		return
	}

	if _, _, err := m.decoder.Decode(body, nil, reviewReq); err != nil {
		klog.Errorf("cant decode body: %s", err)
		m.responseError(w, reviewReq, err)
		return
	}

	// we only mutate pods, if mutating webhook is properly configured this
	// should never happen.
	objkind := reviewReq.Request.Kind.Kind
	if objkind != "Pod" {
		klog.Errorf("strange event for a %s, authorizing", objkind)
		m.responseAuthorized(w, reviewReq)
		return
	}

	var pod corev1.Pod
	if err := json.Unmarshal(reviewReq.Request.Object.Raw, &pod); err != nil {
		klog.Errorf("error decoding raw object: %s", err)
		m.responseError(w, reviewReq, err)
		return
	}

	// XXX namespace comes in empty, set it here.
	pod.Namespace = reviewReq.Request.Namespace

	patch, err := m.tagsvc.PatchForPod(pod)
	if err != nil {
		klog.Errorf("error patching %s: %s", objkind, err)
		m.responseError(w, reviewReq, err)
		return
	}

	var ptype *admnv1.PatchType
	var patchData []byte
	if patch != nil {
		jpt := admnv1.PatchType("JSONPatch")
		ptype = &jpt

		patchData, err = json.Marshal(patch)
		if err != nil {
			klog.Errorf("error marshaling patch: %s", err)
			m.responseError(w, reviewReq, err)
			return
		}
	}

	reviewResp := &admnv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: &admnv1.AdmissionResponse{
			Allowed:   true,
			UID:       reviewReq.Request.UID,
			Patch:     patchData,
			PatchType: ptype,
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

// Start puts the http server online. Requests for resources related to
// deploys (Deployments and Pods) are set to deploy() handler while
// image tag resources are managed by tag() handler.
func (m *MutatingWebHook) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/pod", m.pod)
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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	admnv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	imgtagv1 "github.com/ricardomaraschini/it/imagetags/v1"
	"github.com/ricardomaraschini/it/services"
)

// WebHook handles Mutation requests from kubernetes api.
type WebHook struct {
	key     string
	cert    string
	bind    string
	tagsvc  *services.Tag
	decoder runtime.Decoder
}

// NewWebHook returns a web hook handler for kubernetes api mutation
// requests.
func NewWebHook(tagsvc *services.Tag) *WebHook {
	runtimeScheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(runtimeScheme)
	return &WebHook{
		key:     "assets/server.key",
		cert:    "assets/server.crt",
		bind:    ":8080",
		decoder: codecs.UniversalDeserializer(),
		tagsvc:  tagsvc,
	}
}

// responseError writes on the response an AdmissionReview with response status
// set to an error. If AdmissionReview contains an UID that is inserted into
// the reply.
func (wh *WebHook) responseError(w http.ResponseWriter, req *admnv1.AdmissionReview, err error) {
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
func (wh *WebHook) responseAuthorized(w http.ResponseWriter, req *admnv1.AdmissionReview) {
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
func (wh *WebHook) tag(w http.ResponseWriter, r *http.Request) {
	reviewReq := &admnv1.AdmissionReview{}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		klog.Errorf("error reading body: %s", err)
		wh.responseError(w, reviewReq, err)
		return
	}

	if _, _, err := wh.decoder.Decode(body, nil, reviewReq); err != nil {
		klog.Errorf("cant decoding body: %s", err)
		wh.responseError(w, reviewReq, err)
		return
	}

	// we only mutate pods, if mutating webhook is properly configured this
	// should never happen.
	objkind := reviewReq.Request.Kind.Kind
	if objkind != "Tag" {
		klog.Errorf("received event for %s, authorizing", objkind)
		wh.responseAuthorized(w, reviewReq)
		return
	}

	var tag imgtagv1.Tag
	if err := json.Unmarshal(reviewReq.Request.Object.Raw, &tag); err != nil {
		klog.Errorf("unable to decode tag: %s", err)
		wh.responseError(w, reviewReq, err)
		return
	}

	if err := wh.tagsvc.ValidateTag(tag); err != nil {
		wh.responseError(w, reviewReq, err)
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

// deploy handles mutation requests made by kubernetes api with regards
// to pods and deployments.
func (wh *WebHook) deploy(w http.ResponseWriter, r *http.Request) {
	reviewReq := &admnv1.AdmissionReview{}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		klog.Errorf("error reading body: %s", err)
		wh.responseError(w, reviewReq, err)
		return
	}

	if _, _, err := wh.decoder.Decode(body, nil, reviewReq); err != nil {
		klog.Errorf("cant decoding body: %s", err)
		wh.responseError(w, reviewReq, err)
		return
	}

	// we only mutate pods, if mutating webhook is properly configured this
	// should never happen.
	objkind := reviewReq.Request.Kind.Kind
	if objkind != "Pod" && objkind != "Deployment" {
		klog.Errorf("received event for %s, authorizing", objkind)
		wh.responseAuthorized(w, reviewReq)
		return
	}

	var pod corev1.Pod
	var deploy appsv1.Deployment
	switch objkind {
	case "Pod":
		err = json.Unmarshal(reviewReq.Request.Object.Raw, &pod)
	case "Deployment":
		err = json.Unmarshal(reviewReq.Request.Object.Raw, &deploy)
	}
	if err != nil {
		klog.Errorf("error decoding raw object: %s", err)
		wh.responseError(w, reviewReq, err)
		return
	}

	// XXX namespaces come in empty, set it here.
	pod.Namespace = reviewReq.Request.Namespace
	deploy.Namespace = reviewReq.Request.Namespace

	var patch []byte
	switch objkind {
	case "Pod":
		patch, err = wh.tagsvc.PatchForPod(pod)
	case "Deployment":
		patch, err = wh.tagsvc.PatchForDeployment(deploy)
	}
	if err != nil {
		klog.Errorf("error patching object: %s", err)
		wh.responseError(w, reviewReq, err)
		return
	}

	var ptype *admnv1.PatchType
	if patch != nil {
		jpt := admnv1.PatchType("JSONPatch")
		ptype = &jpt
	}

	reviewResp := &admnv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: &admnv1.AdmissionResponse{
			Allowed:   true,
			UID:       reviewReq.Request.UID,
			Patch:     patch,
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
func (wh *WebHook) Start(ctx context.Context) error {
	http.HandleFunc("/deploy", wh.deploy)
	http.HandleFunc("/tag", wh.tag)
	server := &http.Server{
		Addr: wh.bind,
	}

	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			klog.Errorf("error shutting down https server: %s", err)
		}
	}()

	if err := server.ListenAndServeTLS(wh.cert, wh.key); err != nil {
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
	return nil
}

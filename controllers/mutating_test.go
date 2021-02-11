package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	admnv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/mattbaird/jsonpatch"

	imgv1 "github.com/ricardomaraschini/tagger/infra/tags/v1"
)

type patcher struct {
	err   error
	patch []jsonpatch.JsonPatchOperation
}

func (p *patcher) PatchForPod(pod corev1.Pod) ([]jsonpatch.JsonPatchOperation, error) {
	return p.patch, p.err
}

func Test_responseError(t *testing.T) {
	for _, tt := range []struct {
		name string
		req  *admnv1.AdmissionReview
		code int
		uid  types.UID
	}{
		{
			name: "with request id",
			req: &admnv1.AdmissionReview{
				Request: &admnv1.AdmissionRequest{
					UID: "anuid",
				},
			},
			code: http.StatusOK,
			uid:  "anuid",
		},
		{
			name: "no request id",
			req:  &admnv1.AdmissionReview{},
			code: http.StatusOK,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wr := httptest.NewRecorder()
			mt := NewMutatingWebHook(nil)
			mt.responseError(wr, tt.req, fmt.Errorf("error"))

			if wr.Code != tt.code {
				t.Errorf("expected code %d, %d received", tt.code, wr.Code)
			}

			var rev admnv1.AdmissionReview
			if err := json.NewDecoder(wr.Result().Body).Decode(&rev); err != nil {
				t.Fatalf("error decoding review: %s", err)
			}

			if rev.Response.UID != tt.uid {
				t.Errorf("expected uid %q, %q received", tt.uid, rev.Response.UID)
			}
		})
	}
}

func Test_responseAuthorized(t *testing.T) {
	for _, tt := range []struct {
		name string
		req  *admnv1.AdmissionReview
		code int
		uid  types.UID
	}{
		{
			name: "with request id",
			req: &admnv1.AdmissionReview{
				Request: &admnv1.AdmissionRequest{
					UID: "anuid",
				},
			},
			code: http.StatusOK,
			uid:  "anuid",
		},
		{
			name: "no request id",
			req:  &admnv1.AdmissionReview{},
			code: http.StatusOK,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wr := httptest.NewRecorder()
			mt := NewMutatingWebHook(nil)
			mt.responseAuthorized(wr, tt.req)

			if wr.Code != tt.code {
				t.Errorf("expected code %d, %d received", tt.code, wr.Code)
			}

			var rev admnv1.AdmissionReview
			if err := json.NewDecoder(wr.Result().Body).Decode(&rev); err != nil {
				t.Fatalf("error decoding review: %s", err)
			}

			if rev.Response.UID != tt.uid {
				t.Errorf("expected uid %q, %q received", tt.uid, rev.Response.UID)
			}

			if !rev.Response.Allowed {
				t.Error("request should be allowed")
			}
		})
	}
}

func Test_tag(t *testing.T) {
	for _, tt := range []struct {
		name    string
		kind    string
		tag     *imgv1.Tag
		allowed bool
	}{
		{
			name:    "happy path",
			kind:    "Tag",
			tag:     &imgv1.Tag{},
			allowed: true,
		},
		{
			name:    "invalid kind",
			kind:    "Pod",
			tag:     &imgv1.Tag{},
			allowed: true,
		},
		{
			name:    "invalid tag generation",
			kind:    "Tag",
			allowed: false,
			tag: &imgv1.Tag{
				Spec: imgv1.TagSpec{
					Generation: 10,
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mt := NewMutatingWebHook(nil)

			tagjson, err := json.Marshal(tt.tag)
			if err != nil {
				t.Fatalf("error marshaling tag: %s", err)
			}

			req := admnv1.AdmissionReview{
				Request: &admnv1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Kind: tt.kind,
					},
					Object: runtime.RawExtension{
						Object: tt.tag,
						Raw:    tagjson,
					},
					UID: types.UID(tt.name),
				},
			}

			buf := bytes.NewBuffer(nil)
			if err := json.NewEncoder(buf).Encode(req); err != nil {
				t.Fatalf("error marshaling body: %s", err)
			}

			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/tag", buf)
			mt.tag(w, r)
			defer r.Body.Close()

			var resp admnv1.AdmissionReview
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("error decoding reply: %s", err)
			}

			if resp.Response.UID != types.UID(tt.name) {
				t.Fatalf("expected uid %q, %q found", tt.name, resp.Response.UID)
			}

			if resp.Response.Allowed != tt.allowed {
				t.Fatalf("expected allowed to be %v", tt.allowed)
			}
		})
	}
}

func Test_pod(t *testing.T) {
	for _, tt := range []struct {
		name         string
		kind         string
		patcherError error
		patcherPatch []jsonpatch.JsonPatchOperation
		pod          *corev1.Pod
		allowed      bool
	}{
		{
			name:    "happy path",
			kind:    "Pod",
			pod:     &corev1.Pod{},
			allowed: true,
		},
		{
			name:    "happy path with patch",
			kind:    "Pod",
			allowed: true,
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Hostname: "hostname",
				},
			},
			patcherPatch: []jsonpatch.JsonPatchOperation{
				{
					Operation: "replace",
					Path:      "spec/hostname",
					Value:     "newhostname",
				},
			},
		},
		{
			name:    "wrong kind",
			kind:    "Deployment",
			pod:     &corev1.Pod{},
			allowed: true,
		},
		{
			name:         "error creating patch",
			kind:         "Pod",
			patcherError: fmt.Errorf("an error"),
			allowed:      false,
			pod:          &corev1.Pod{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mt := NewMutatingWebHook(&patcher{
				err:   tt.patcherError,
				patch: tt.patcherPatch,
			})

			podjson, err := json.Marshal(tt.pod)
			if err != nil {
				t.Fatalf("error marshaling pod: %s", err)
			}

			req := admnv1.AdmissionReview{
				Request: &admnv1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Kind: tt.kind,
					},
					Object: runtime.RawExtension{
						Object: tt.pod,
						Raw:    podjson,
					},
					UID: types.UID(tt.name),
				},
			}

			buf := bytes.NewBuffer(nil)
			if err := json.NewEncoder(buf).Encode(req); err != nil {
				t.Fatalf("error marshaling body: %s", err)
			}

			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/pod", buf)
			mt.pod(w, r)
			defer r.Body.Close()

			var resp admnv1.AdmissionReview
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("error decoding reply: %s", err)
			}

			if resp.Response.UID != types.UID(tt.name) {
				t.Errorf("expected uid %q, %q found", tt.name, resp.Response.UID)
			}

			if resp.Response.Allowed != tt.allowed {
				t.Errorf("expected allowed to be %v", tt.allowed)
			}

			if tt.patcherPatch == nil {
				if len(resp.Response.Patch) > 0 {
					t.Errorf("unexpected patch: %+v", resp.Response.Patch)
				}
				return
			}

			encpatch, err := json.Marshal(tt.patcherPatch)
			if err != nil {
				t.Fatalf("unable to marshal patch: %s", err)
			}

			if !reflect.DeepEqual(encpatch, resp.Response.Patch) {
				t.Errorf("unexpected patch %s", string(resp.Response.Patch))
			}
		})
	}
}

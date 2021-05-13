package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	admnv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	imgv1beta1 "github.com/ricardomaraschini/tagger/infra/tags/v1beta1"
)

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
			mt := NewMutatingWebHook()
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
			mt := NewMutatingWebHook()
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
		tag     *imgv1beta1.Tag
		allowed bool
	}{
		{
			name:    "happy path",
			kind:    "Tag",
			tag:     &imgv1beta1.Tag{},
			allowed: true,
		},
		{
			name:    "invalid kind",
			kind:    "Pod",
			tag:     &imgv1beta1.Tag{},
			allowed: true,
		},
		{
			name:    "invalid tag generation",
			kind:    "Tag",
			allowed: false,
			tag: &imgv1beta1.Tag{
				Spec: imgv1beta1.TagSpec{
					Generation: 10,
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mt := NewMutatingWebHook()

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

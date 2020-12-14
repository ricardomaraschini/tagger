package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	admnv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/types"
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

// Copyright 2020 The Tagger Authors.
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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	admnv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	imgv1beta1 "github.com/ricardomaraschini/tagger/infra/images/v1beta1"
)

type imgImportValidator struct{}

func (t imgImportValidator) Validate(_ context.Context, ti *imgv1beta1.ImageImport) error {
	return nil
}

type imgValidator struct{}

func (t imgValidator) Validate(_ context.Context, ti *imgv1beta1.Image) error {
	return nil
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
			mt := NewMutatingWebHook(imgImportValidator{}, imgValidator{})
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
			mt := NewMutatingWebHook(imgImportValidator{}, imgValidator{})
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

func Test_image(t *testing.T) {
	for _, tt := range []struct {
		name    string
		kind    string
		img     *imgv1beta1.Image
		allowed bool
	}{
		{
			name:    "happy path",
			kind:    "Image",
			img:     &imgv1beta1.Image{},
			allowed: true,
		},
		{
			name:    "invalid kind",
			kind:    "Pod",
			img:     &imgv1beta1.Image{},
			allowed: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mt := NewMutatingWebHook(imgImportValidator{}, imgValidator{})

			imgjson, err := json.Marshal(tt.img)
			if err != nil {
				t.Fatalf("error marshaling img: %s", err)
			}

			req := admnv1.AdmissionReview{
				Request: &admnv1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Kind: tt.kind,
					},
					Object: runtime.RawExtension{
						Object: tt.img,
						Raw:    imgjson,
					},
					UID: types.UID(tt.name),
				},
			}

			buf := bytes.NewBuffer(nil)
			if err := json.NewEncoder(buf).Encode(req); err != nil {
				t.Fatalf("error marshaling body: %s", err)
			}

			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/image", buf)
			mt.image(w, r)
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

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
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestDockerWebHooks(t *testing.T) {
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	svc := &tagupdater{}
	srv := NewDockerWebHook(svc)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Start(ctx); err != nil {
			t.Errorf("error reported by srv.Start: %s", err)
		}
	}()

	// give it some time for the http server to be online.
	time.Sleep(time.Second)

	for _, tt := range []struct {
		name       string
		reqbody    interface{}
		expected   []string
		statuscode int
		errorout   bool
	}{
		{
			name: "happy path",
			reqbody: map[string]interface{}{
				"push_data": map[string]interface{}{
					"tag": "latest",
				},
				"repository": map[string]interface{}{
					"namespace": "tagger",
					"name":      "app",
				},
			},
			expected:   []string{"docker.io/tagger/app:latest"},
			statuscode: http.StatusOK,
		},
		{
			name: "no tag",
			reqbody: map[string]interface{}{
				"repository": map[string]interface{}{
					"namespace": "tagger",
					"name":      "app",
				},
			},
			expected:   nil,
			statuscode: http.StatusBadRequest,
		},
		{
			name: "no name",
			reqbody: map[string]interface{}{
				"push_data": map[string]interface{}{
					"tag": "latest",
				},
				"repository": map[string]interface{}{
					"namespace": "tagger",
				},
			},
			expected:   nil,
			statuscode: http.StatusBadRequest,
		},
		{
			name: "no namespace",
			reqbody: map[string]interface{}{
				"push_data": map[string]interface{}{
					"tag": "latest",
				},
				"repository": map[string]interface{}{
					"name": "app",
				},
			},
			expected:   nil,
			statuscode: http.StatusBadRequest,
		},
		{
			name: "error on service",
			reqbody: map[string]interface{}{
				"push_data": map[string]interface{}{
					"tag": "latest",
				},
				"repository": map[string]interface{}{
					"namespace": "tagger",
					"name":      "app",
				},
			},
			errorout:   true,
			expected:   nil,
			statuscode: http.StatusInternalServerError,
		},
		{
			name:       "error decoding",
			reqbody:    "<--xyk",
			errorout:   true,
			expected:   nil,
			statuscode: http.StatusBadRequest,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc.errorout = tt.errorout

			buf := bytes.NewBuffer(nil)
			if err := json.NewEncoder(buf).Encode(tt.reqbody); err != nil {
				t.Fatalf("error marshaling body: %s", err)
			}

			res, err := http.Post("http://localhost:8082", "application/json", buf)
			if err != nil {
				t.Fatalf("error requesting: %s", err)
			}
			defer res.Body.Close()

			if res.StatusCode != tt.statuscode {
				t.Errorf("wrong status code returned: %d", res.StatusCode)
			}

			if !reflect.DeepEqual(tt.expected, svc.imgpaths) {
				t.Errorf("expected %+v, found %+v", tt.expected, svc.imgpaths)
			}
			svc.imgpaths = nil
		})
	}

	cancel()
	wg.Wait()
}

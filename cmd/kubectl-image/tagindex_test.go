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

package main

import (
	"reflect"
	"strings"
	"testing"
)

func Test_indexFor(t *testing.T) {
	for _, tt := range []struct {
		name   string
		ipath  string
		experr string
		expidx imageindex
	}{
		{
			name:   "clean",
			ipath:  "a.path/namespace/name",
			experr: "",
			expidx: imageindex{
				server:    "a.path",
				namespace: "namespace",
				name:      "name",
			},
		},
		{
			name:   "with a tag extension",
			ipath:  "a.path/namespace/name:atag",
			experr: "",
			expidx: imageindex{
				server:    "a.path",
				namespace: "namespace",
				name:      "name",
			},
		},
		{
			name:   "by hash",
			ipath:  "a.path/namespace/name@sha245:abcdef",
			experr: "",
			expidx: imageindex{
				server:    "a.path",
				namespace: "namespace",
				name:      "name",
			},
		},
		{
			name:   "using ip address by hash",
			ipath:  "10.1.1.1:8083/namespace/name@sha245:abcdef",
			experr: "",
			expidx: imageindex{
				server:    "10.1.1.1:8083",
				namespace: "namespace",
				name:      "name",
			},
		},
		{
			name:   "using ip address by tag",
			ipath:  "10.1.1.1:8083/namespace/name:latest",
			experr: "",
			expidx: imageindex{
				server:    "10.1.1.1:8083",
				namespace: "namespace",
				name:      "name",
			},
		},
		{
			name:   "invalid image path",
			ipath:  "apathnamespacename",
			experr: "unexpected image path",
			expidx: imageindex{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tidx, err := indexFor(tt.ipath)
			if len(tt.experr) > 0 {
				if !strings.Contains(err.Error(), tt.experr) {
					t.Errorf("expected err to contain %q: %q", tt.experr, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if !reflect.DeepEqual(tt.expidx, tidx) {
				t.Errorf("expected %+v, received %+v", tt.expidx, tidx)
			}
		})
	}
}

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
		expidx tagindex
	}{
		{
			name:   "clean",
			ipath:  "a.path/namespace/name",
			experr: "",
			expidx: tagindex{
				server:    "a.path",
				namespace: "namespace",
				name:      "name",
			},
		},
		{
			name:   "with a tag extension",
			ipath:  "a.path/namespace/name:atag",
			experr: "",
			expidx: tagindex{
				server:    "a.path",
				namespace: "namespace",
				name:      "name",
			},
		},
		{
			name:   "by hash",
			ipath:  "a.path/namespace/name@sha245:abcdef",
			experr: "",
			expidx: tagindex{
				server:    "a.path",
				namespace: "namespace",
				name:      "name",
			},
		},
		{
			name:   "using ip address by hash",
			ipath:  "10.1.1.1:8083/namespace/name@sha245:abcdef",
			experr: "",
			expidx: tagindex{
				server:    "10.1.1.1:8083",
				namespace: "namespace",
				name:      "name",
			},
		},
		{
			name:   "using ip address by tag",
			ipath:  "10.1.1.1:8083/namespace/name:latest",
			experr: "",
			expidx: tagindex{
				server:    "10.1.1.1:8083",
				namespace: "namespace",
				name:      "name",
			},
		},
		{
			name:   "invalid image path",
			ipath:  "apathnamespacename",
			experr: "unexpected image path",
			expidx: tagindex{},
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

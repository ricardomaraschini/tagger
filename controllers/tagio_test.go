package controllers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"
)

type tagexporter struct {
	fail bool
	body []byte
}

func (t *tagexporter) Export(ctx context.Context, ns, name string) (io.ReadCloser, error) {
	if t.fail {
		return nil, fmt.Errorf("error exporting tag")
	}
	return ioutil.NopCloser(bytes.NewBuffer(t.body)), nil
}

type uservalidator struct {
	allowed bool
}

func (u *uservalidator) CanAccessTag(context.Context, string, string) error {
	if !u.allowed {
		return fmt.Errorf("not allowed")
	}
	return nil
}

func TestTagIO(t *testing.T) {
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	expsvc := &tagexporter{}
	usrsvc := &uservalidator{}
	srv := NewTagIO(expsvc, usrsvc)
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
		allowed    bool
		exportfail bool
		exportbody []byte
		statuscode int
		token      string
		path       string
	}{
		{
			name:       "no token",
			statuscode: http.StatusBadRequest,
			exportbody: []byte(""),
			path:       "/?namespace=ns&name=name",
		},
		{
			name:       "no namespace",
			statuscode: http.StatusBadRequest,
			allowed:    true,
			exportbody: []byte(""),
			path:       "/?name=name",
			token:      "a-token",
		},
		{
			name:       "no name",
			statuscode: http.StatusBadRequest,
			allowed:    true,
			exportbody: []byte(""),
			path:       "/?&namespace=name",
			token:      "a-token",
		},
		{
			name:       "not allowed",
			statuscode: http.StatusForbidden,
			allowed:    false,
			exportbody: []byte(""),
			path:       "/?namespace=ns&name=name",
			token:      "a-token",
		},
		{
			name:       "error exporting",
			statuscode: http.StatusInternalServerError,
			allowed:    true,
			exportfail: true,
			exportbody: []byte(""),
			path:       "/?namespace=ns&name=name",
			token:      "a-token",
		},
		{
			name:       "happy path",
			statuscode: http.StatusOK,
			allowed:    true,
			exportbody: []byte("123"),
			path:       "/?namespace=ns&name=name",
			token:      "a-token",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			usrsvc.allowed = tt.allowed
			expsvc.fail = tt.exportfail
			expsvc.body = tt.exportbody

			url := fmt.Sprintf("http://localhost:8083/%s", tt.path)
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				t.Fatalf("unexpected error creating request: %s", err)
			}

			if len(tt.token) > 0 {
				tokenhdr := fmt.Sprintf("Bearer %s", tt.token)
				req.Header.Add("Authorization", tokenhdr)
			}

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("error requesting: %s", err)
			}
			defer res.Body.Close()

			if res.StatusCode != tt.statuscode {
				t.Errorf("wrong status code returned: %d", res.StatusCode)
			}

			out, err := ioutil.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("unexpected error reading response: %s", err)
			}

			if !reflect.DeepEqual(tt.exportbody, out) {
				t.Errorf("expected %+v, found %+v", tt.exportbody, out)
			}
		})
	}

	cancel()
	wg.Wait()
}

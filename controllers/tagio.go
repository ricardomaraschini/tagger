package controllers

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// TagExporter is here to make tests easier. You may be looking for its
// concrete implementation in services/tag.go.
type TagExporter interface {
	Export(context.Context, string, string) (io.ReadCloser, error)
}

// UserValidator validates an user can access Tags in a given namespace.
// You should be looking for a concrete implementation of this, please
// look at services/user.go and you will find it.
type UserValidator interface {
	CanAccessTag(context.Context, string, string) error
}

// TagIO handles requests for exporting and import Tag custom resources.
type TagIO struct {
	bind   string
	tagexp TagExporter
	usrval UserValidator
}

// NewTagIO returns a web hook handler for quay webhooks.
func NewTagIO(
	tagexp TagExporter,
	usrval UserValidator,
) *TagIO {
	return &TagIO{
		bind:   ":8083",
		tagexp: tagexp,
		usrval: usrval,
	}
}

// Name returns a name identifier for this controller.
func (t *TagIO) Name() string {
	return "tag input/output webhook"
}

// ServeHTTP handles requests coming in.
func (t *TagIO) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tkheader := r.Header.Get("Authorization")
	tkslices := strings.Split(tkheader, "Bearer ")
	if len(tkslices) != 2 {
		klog.Errorf("invalid token header length: %d", len(tkslices))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	token := tkslices[1]

	query := r.URL.Query()
	namespaces, ok := query["namespace"]
	if !ok || len(namespaces) != 1 {
		klog.Errorf("tag namespace not set")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	namespace := namespaces[0]

	names, ok := query["name"]
	if !ok || len(names) != 1 {
		klog.Errorf("tag name not set")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	name := names[0]

	if err := t.usrval.CanAccessTag(r.Context(), namespace, token); err != nil {
		klog.Errorf("user cannot access tags in namespace: %s", err)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	fp, err := t.tagexp.Export(r.Context(), namespace, name)
	if err != nil {
		klog.Errorf("unexpected error exporting tag: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer fp.Close()

	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, fp); err != nil {
		klog.Errorf("error copying tag: %s", err)
	}
}

// Start puts the http server online.
func (t *TagIO) Start(ctx context.Context) error {
	server := &http.Server{
		Addr:    t.bind,
		Handler: t,
	}

	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			klog.Errorf("error shutting down http server: %s", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil {
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
	return nil
}

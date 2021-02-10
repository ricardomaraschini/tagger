package controllers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// TagImporterExporter is here to make tests easier. You may be looking for
// its concrete implementation in services/tagio.go.
type TagImporterExporter interface {
	Import(context.Context, string, string, io.Reader) error
	Export(context.Context, string, string) (io.ReadCloser, func(), error)
}

// UserValidator validates an user can access Tags in a given namespace.
// You should be looking for a concrete implementation of this, please
// look at services/user.go and you will find it.
type UserValidator interface {
	CanAccessTags(context.Context, string, string) error
}

// TagIO handles requests for exporting and import Tag custom resources.
type TagIO struct {
	bind   string
	tagexp TagImporterExporter
	usrval UserValidator
}

// NewTagIO returns a web hook handler for quay webhooks.
func NewTagIO(
	tagexp TagImporterExporter,
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
	return "tag input/output handler"
}

// ServeHTTP handles requests coming in. Validates if the request contains
// a valid kubernetes token through an UserValidator call and then based
// on the request method disptaches an import (post) or export (get).
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

	// checks if the token allows access to tags in the provided namespace.
	if err := t.usrval.CanAccessTags(r.Context(), namespace, token); err != nil {
		klog.Errorf("user cannot access tags in namespace: %s", err)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodGet:
		t.exportTag(namespace, name, w, r)
	case http.MethodPost:
		t.importTag(namespace, name, w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// importTag reads content from the request body and attempts to uncompress it,
// creating a tag if succeeds.
func (t *TagIO) importTag(namespace, name string, w http.ResponseWriter, r *http.Request) {
	if err := t.tagexp.Import(r.Context(), namespace, name, r.Body); err != nil {
		klog.Errorf("unexpected error importing tag: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// exportTag compresses a tag and writes the content down into the writer.
func (t *TagIO) exportTag(namespace, name string, w http.ResponseWriter, r *http.Request) {
	fp, cleanup, err := t.tagexp.Export(r.Context(), namespace, name)
	if err != nil {
		klog.Errorf("unexpected error exporting tag: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer cleanup()

	hdr := fmt.Sprintf("attachment; filename=tag-%s-%s.tar.gz", namespace, name)
	w.Header().Set("Content-Disposition", hdr)
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

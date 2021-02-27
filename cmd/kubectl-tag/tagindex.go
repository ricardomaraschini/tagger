package main

import (
	"fmt"
	"strings"

	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
)

// tagindex provides identification for a single tag.
type tagindex struct {
	server    string
	namespace string
	name      string
}

// localref returns an ImageReference pointing to the local storage.
func (t tagindex) localref() (types.ImageReference, error) {
	str := fmt.Sprintf(
		"containers-storage:%s/%s/%s:latest",
		t.server, t.namespace, t.name,
	)
	return alltransports.ParseImageName(str)
}

// indexFor receives a path to an image hosted at a tagger instance
// and constructs a tagindex by parsing it.
func indexFor(ipath string) (tagindex, error) {
	var zero tagindex

	slices := strings.SplitN(ipath, "/", 3)
	if len(slices) < 3 {
		return zero, fmt.Errorf("unexpected image path layout")
	}

	tidx := tagindex{
		server:    slices[0],
		namespace: slices[1],
		name:      slices[2],
	}

	// this is a different way of adressing images, but by appending
	// an "@sha256:" after the image name. we also ignore it.
	slices = strings.SplitN(tidx.name, "@", 2)
	if len(slices) == 2 {
		tidx.name = slices[0]
		return tidx, nil
	}

	// user has entered with "tag" name on the image path, something
	// like "tagger.addr:port/namespace/name:tag", this "tag" portion
	// is ignored, we only care about the namespace and the name.
	slices = strings.SplitN(tidx.name, ":", 2)
	if len(slices) == 2 {
		tidx.name = slices[0]
		return tidx, nil
	}

	return tidx, nil
}

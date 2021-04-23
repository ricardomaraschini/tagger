package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/hashicorp/go-multierror"
)

// List of container runtimes.
const (
	UnknownRuntime = iota
	PodmanRuntime
	DockerRuntime
)

// tagindex provides identification for a single tag. Here 'server' is the
// tagger url (i.e. the cluster), namespace and name uniquely identify a
// tag on the cluster.
type tagindex struct {
	server    string
	namespace string
	name      string
}

// containerRuntime returns the container runtime installed in the operating
// system. We do an attempt to look for a binary called 'podman' or 'docker'
// in the host PATH environment variable.
func (t tagindex) containerRuntime() (int, error) {
	var errors *multierror.Error

	_, err := exec.LookPath("podman")
	if err == nil {
		return PodmanRuntime, nil
	}
	errors = multierror.Append(errors, err)

	_, err = exec.LookPath("docker")
	if err == nil {
		return DockerRuntime, nil
	}
	errors = multierror.Append(errors, err)

	err = fmt.Errorf("unable to determine container runtime: %w", errors)
	return UnknownRuntime, err
}

// localStorageRef returns an ImageReference pointing to the local storage.
// The returned reference points or to podman or docker storage, according
// to what is installed in the system.
func (t tagindex) localStorageRef() (types.ImageReference, error) {
	runtime, err := t.containerRuntime()
	if err != nil {
		return nil, err
	}

	transport := "containers-storage"
	if runtime == DockerRuntime {
		transport = "docker-daemon"
	}

	str := fmt.Sprintf(
		"%s:%s/%s/%s:latest",
		transport, t.server, t.namespace, t.name,
	)
	return alltransports.ParseImageName(str)
}

// indexFor receives a path to an image hosted at a tagger instance
// and constructs a tagindex by parsing it.
func indexFor(ipath string) (tagindex, error) {
	var zero tagindex

	// we expect the path to be at least "server:port/namespace/name".
	slices := strings.SplitN(ipath, "/", 3)
	if len(slices) < 3 {
		return zero, fmt.Errorf("unexpected image path layout")
	}

	tidx := tagindex{
		server:    slices[0],
		namespace: slices[1],
		name:      slices[2],
	}

	// user has requested a tag by its hash, we don't care about the
	// hash part, we only care about the namespace and the name.
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

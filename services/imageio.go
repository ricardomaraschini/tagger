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

package services

import (
	"context"
	"fmt"
	"os"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/utils/pointer"

	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"

	imgclient "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/clientset/versioned"
	imginform "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/informers/externalversions"
	imglist "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/listers/images/v1beta1"
	"github.com/ricardomaraschini/tagger/infra/metrics"
)

// ImageIO is an entity that gather operations related to Image images input and
// output. This entity allow users to pull images from or to push images to
// a Image.
type ImageIO struct {
	imgcli imgclient.Interface
	imglis imglist.ImageLister
	syssvc *SysContext
}

// NewImageIO returns a new ImageIO object, capable of import and export Image images.
func NewImageIO(
	corinf informers.SharedInformerFactory,
	imgcli imgclient.Interface,
	imginf imginform.SharedInformerFactory,
) *ImageIO {
	var imglis imglist.ImageLister
	if imginf != nil {
		imglis = imginf.Tagger().V1beta1().Images().Lister()
	}

	return &ImageIO{
		imgcli: imgcli,
		imglis: imglis,
		syssvc: NewSysContext(corinf),
	}
}

// Push expects "fpath" to point to a valid docker image stored on disk as a tar
// file, reads it and then pushes it to our mirror registry through an image store
// implementation (see infra/imagestore/registry.go for a concrete implementation).
func (t *ImageIO) Push(ctx context.Context, ns, name string, fpath string) error {
	start := time.Now()

	var worked bool
	defer func() {
		if !worked {
			metrics.PushFailures.Inc()
			return
		}
		latency := time.Now().Sub(start).Seconds()
		metrics.PushLatency.Observe(latency)
		metrics.PushSuccesses.Inc()
	}()

	istore, err := t.syssvc.GetRegistryStore(ctx)
	if err != nil {
		return fmt.Errorf("error creating image store: %w", err)
	}

	refstr := fmt.Sprintf("docker-archive://%s", fpath)
	srcref, err := alltransports.ParseImageName(refstr)
	if err != nil {
		return fmt.Errorf("error parsing image name: %w", err)
	}

	// we pass nil as source context reference as to read the file from disk
	// no authentication is needed. Namespace is used as repository and
	// name as image name.
	dstref, err := istore.Load(ctx, srcref, nil, ns, name)
	if err != nil {
		return fmt.Errorf("error loading image into registry: %w", err)
	}

	regctx := t.syssvc.MirrorRegistryContext(ctx)
	insecure := regctx.DockerInsecureSkipTLSVerify == types.OptionalBoolTrue

	opts := ImportOpts{
		Namespace:   ns,
		TargetImage: name,
		From:        dstref.DockerReference().String(),
		Mirror:      pointer.Bool(false),
		Insecure:    pointer.Bool(insecure),
	}

	tisvc := NewImageImport(nil, t.imgcli, nil)
	if _, err := tisvc.NewImport(ctx, opts); err != nil {
		return fmt.Errorf("unable to create image import: %w", err)
	}

	worked = true
	return nil
}

// Pull saves a Image image into a tar file and returns a reader from where the image
// content can be read. Caller is responsible for cleaning up after the returned
// resources by calling the returned function.
func (t *ImageIO) Pull(ctx context.Context, ns, name string) (*os.File, func(), error) {
	start := time.Now()

	var worked bool
	defer func() {
		if !worked {
			metrics.PullFailures.Inc()
			return
		}

		latency := time.Now().Sub(start).Seconds()
		metrics.PullLatency.Observe(latency)
		metrics.PullSuccesses.Inc()
	}()

	it, err := t.imglis.Images(ns).Get(name)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting image: %w", err)
	}

	istore, err := t.syssvc.GetRegistryStore(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating image store: %w", err)
	}

	imgref := it.CurrentReferenceForImage()
	if len(imgref) == 0 {
		return nil, nil, fmt.Errorf("reference for current generation not found")
	}

	from := fmt.Sprintf("docker://%s", imgref)
	fromRef, err := alltransports.ParseImageName(from)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing image reference: %w", err)
	}

	toRef, cleanup, err := istore.Save(ctx, fromRef)
	if err != nil {
		return nil, nil, fmt.Errorf("error saving image locally: %w", err)
	}
	fpath := toRef.StringWithinTransport()

	fp, err := os.Open(fpath)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("error opening tar file: %w", err)
	}
	ncleanup := func() {
		fp.Close()
		cleanup()
	}

	worked = true
	return fp, ncleanup, nil
}

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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"

	"github.com/containers/image/v5/transports/alltransports"

	"github.com/ricardomaraschini/tagger/infra/metrics"
	imagtagv1beta1 "github.com/ricardomaraschini/tagger/infra/tags/v1beta1"
	tagclient "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/clientset/versioned"
	taginform "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/informers/externalversions"
	taglist "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/listers/tags/v1beta1"
)

// TagIO is an entity that gather operations related to Tag images input and
// output. This entity allow users to pull images from or to push images to
// a Tag.
type TagIO struct {
	tagcli tagclient.Interface
	taglis taglist.TagLister
	syssvc *SysContext
}

// NewTagIO returns a new TagIO object, capable of import and export Tag images.
func NewTagIO(
	corinf informers.SharedInformerFactory,
	tagcli tagclient.Interface,
	taginf taginform.SharedInformerFactory,
) *TagIO {
	var taglis taglist.TagLister
	if taginf != nil {
		taglis = taginf.Tagger().V1beta1().Tags().Lister()
	}

	return &TagIO{
		tagcli: tagcli,
		taglis: taglis,
		syssvc: NewSysContext(corinf),
	}
}

// tagOrNew returns or an existing Tag or a new one. Returned tag
// is configured with 'mirror' set as true.
func (t *TagIO) tagOrNew(ns, name string) (*imagtagv1beta1.Tag, error) {
	it, err := t.taglis.Tags(ns).Get(name)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}

	if err == nil {
		return it, nil
	}

	return &imagtagv1beta1.Tag{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: imagtagv1beta1.TagSpec{
			Mirror: true,
		},
	}, nil
}

// Push expects "fpath" to point to a valid docker image stored on disk as a tar
// file, reads it and then pushes it to our mirror registry through an image store
// implementation (see infra/imagestore/registry.go for a concrete implementation).
func (t *TagIO) Push(ctx context.Context, ns, name string, fpath string) error {
	metrics.ImagePushes.Inc()
	istore, err := t.syssvc.GetRegistryStore(ctx)
	if err != nil {
		return fmt.Errorf("error creating image store: %w", err)
	}

	it, err := t.tagOrNew(ns, name)
	if err != nil {
		return fmt.Errorf("error getting tag: %w", err)
	}
	isnew := it.Spec.From == ""

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

	it.Spec.From = dstref.DockerReference().String()
	it.Spec.Generation = it.NextGeneration()
	if isnew {
		_, err = t.tagcli.TaggerV1beta1().Tags(ns).Create(
			ctx, it, metav1.CreateOptions{},
		)
		return err
	}

	if it.Spec.SignOnPush {
		if err := istore.Sign(ctx, dstref); err != nil {
			return fmt.Errorf("error signing image: %w", err)
		}
	}

	_, err = t.tagcli.TaggerV1beta1().Tags(ns).Update(
		ctx, it, metav1.UpdateOptions{},
	)
	return err
}

// Pull saves a Tag image into a tar file and returns a reader from where the image
// content can be read. Caller is responsible for cleaning up after the returned
// resources by calling the returned function.
func (t *TagIO) Pull(ctx context.Context, ns, name string) (*os.File, func(), error) {
	metrics.ImagePulls.Inc()
	it, err := t.taglis.Tags(ns).Get(name)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting tag: %w", err)
	}

	istore, err := t.syssvc.GetRegistryStore(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating image store: %w", err)
	}

	imgref := it.CurrentReferenceForTag()
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
	return fp, ncleanup, nil
}

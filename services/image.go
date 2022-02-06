// Copyright 2020 The Imageger Authors.
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
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	imgv1b1 "github.com/ricardomaraschini/tagger/infra/images/v1beta1"
	imgclient "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/clientset/versioned"
	imginform "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/informers/externalversions"
	imglist "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/listers/images/v1beta1"
)

// Image gather all actions related to image img objects.
type Image struct {
	imgcli imgclient.Interface
	imglis imglist.ImageLister
	implis imglist.ImageImportLister
	imginf imginform.SharedInformerFactory
	syssvc *SysContext
}

// NewImage returns a handler for all image img related services. I have chosen to go with a lazy
// approach here, you can pass or omit (nil) any parameter, it is up to the caller to decide what
// is needed for each specific case.
func NewImage(
	corinf informers.SharedInformerFactory,
	imgcli imgclient.Interface,
	imginf imginform.SharedInformerFactory,
) *Image {
	var imglis imglist.ImageLister
	var implis imglist.ImageImportLister
	if imginf != nil {
		imglis = imginf.Tagger().V1beta1().Images().Lister()
		implis = imginf.Tagger().V1beta1().ImageImports().Lister()
	}

	return &Image{
		imginf: imginf,
		imgcli: imgcli,
		imglis: imglis,
		implis: implis,
		syssvc: NewSysContext(corinf),
	}
}

// RecentlyFinishedImports return all ImageImport objects that refer to provided Image and have
// been processed since the last import found in provided Image.Status.HashReferences. They are
// returned in a sorted slice, from the oldest to the newest.
func (t *Image) RecentlyFinishedImports(
	ctx context.Context, it *imgv1b1.Image,
) ([]imgv1b1.ImageImport, error) {
	imports, err := t.implis.ImageImports(it.Namespace).List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list images: %w", err)
	}

	// remove all ImageImport not imported yet, due to failure or due to delays.
	var sortme []imgv1b1.ImageImport
	for _, imp := range imports {
		if imp.Spec.TargetImage != it.Name {
			continue
		}
		if !imp.AlreadyImported() {
			continue
		}

		// do not return anything that has already been catalogued in the
		// Image status references.
		if len(it.Status.HashReferences) > 0 {
			lastimport := it.Status.HashReferences[0].ImportedAt.Time
			importtime := imp.Status.HashReference.ImportedAt.Time
			if lastimport.After(importtime) || lastimport.Equal(importtime) {
				continue
			}
		}

		impptr := imp.DeepCopy()
		sortme = append(sortme, *impptr)
	}

	sort.SliceStable(
		sortme,
		func(i, j int) bool {
			first := sortme[i].Status.HashReference.ImportedAt.Time
			second := sortme[j].Status.HashReference.ImportedAt.Time
			return second.After(first)
		},
	)

	sorted := sortme // :-X
	return sorted, nil
}

// Sync manages image updates, assuring we have the image imported.  Beware that we change Image
// in place before updating it on api server, i.e. use DeepCopy() before passing the image object
// in.
func (t *Image) Sync(ctx context.Context, it *imgv1b1.Image) error {
	var err error

	newimports, err := t.RecentlyFinishedImports(ctx, it)
	if err != nil {
		return fmt.Errorf("unable to read image imports: %w", err)
	}

	it.PrependFinishedImports(newimports)

	if _, err = t.imgcli.TaggerV1beta1().Images(it.Namespace).UpdateStatus(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return fmt.Errorf("error updating image: %w", err)
	}
	return nil
}

// Get returns a Image object. Returned object is already a copy of the cached object and may be
// modified by caller as needed.
func (t *Image) Get(ctx context.Context, ns, name string) (*imgv1b1.Image, error) {
	img, err := t.imglis.Images(ns).Get(name)
	if err != nil {
		return nil, fmt.Errorf("unable to get image: %w", err)
	}
	return img.DeepCopy(), nil
}

// Validate checks if provided Image contains all mandatory fields. At this stage we only verify
// if it contain the necessary fields.
func (t *Image) Validate(ctx context.Context, it *imgv1b1.Image) error {
	return it.Validate()
}

// AddEventHandler adds a handler to Image related events.
func (t *Image) AddEventHandler(handler cache.ResourceEventHandler) {
	t.imginf.Tagger().V1beta1().Images().Informer().AddEventHandler(handler)
}

// NewImageOpts holds the options necessary to call Image.NewImage().
type NewImageOpts struct {
	Namespace string
	Name      string
	From      string
	Mirror    bool
	Insecure  bool
}

// NewImage creates and saves a new Image object. Saves it to kubernetes api before returning.
func (t *Image) NewImage(ctx context.Context, o NewImageOpts) (*imgv1b1.Image, error) {
	it := &imgv1b1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name: o.Name,
		},
		Spec: imgv1b1.ImageSpec{
			From:     o.From,
			Mirror:   o.Mirror,
			Insecure: o.Insecure,
		},
	}
	return t.imgcli.TaggerV1beta1().Images(o.Namespace).Create(ctx, it, metav1.CreateOptions{})
}

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
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/hashicorp/go-multierror"

	"github.com/ricardomaraschini/tagger/infra/metrics"
	imagtagv1beta1 "github.com/ricardomaraschini/tagger/infra/tags/v1beta1"
	tagclient "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/clientset/versioned"
	taginform "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/informers/externalversions"
	taglist "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/listers/tags/v1beta1"
)

// Tag gather all actions related to image tag objects.
type Tag struct {
	tagcli tagclient.Interface
	taglis taglist.TagLister
	taginf taginform.SharedInformerFactory
	syssvc *SysContext
}

// NewTag returns a handler for all image tag related services. I have chosen to
// go with a lazy approach here, you can pass or omit (nil) any parameter, it is
// up to the caller to decide what is needed for each specific case. So far this
// is the best approach, I still plan to review this.
func NewTag(
	corinf informers.SharedInformerFactory,
	tagcli tagclient.Interface,
	taginf taginform.SharedInformerFactory,
) *Tag {
	var taglis taglist.TagLister
	if taginf != nil {
		taglis = taginf.Tagger().V1beta1().Tags().Lister()
	}

	return &Tag{
		taginf: taginf,
		tagcli: tagcli,
		taglis: taglis,
		syssvc: NewSysContext(corinf),
	}
}

// Sync manages image tag updates, assuring we have the tag imported.
// Beware that we change Tag in place before updating it on api server,
// i.e. use DeepCopy() before passing the image tag in.
func (t *Tag) Sync(ctx context.Context, it *imagtagv1beta1.Tag) error {
	var err error
	var hashref imagtagv1beta1.HashReference

	alreadyImported := it.SpecTagImported()
	if !alreadyImported {
		klog.Infof("tag %s/%s needs import, importing...", it.Namespace, it.Name)

		hashref, err = t.ImportTag(ctx, it)
		if err != nil {
			// if we fail to import the tag we need to record the failure on tag's
			// status and update it. If we fail to update the tag we only log,
			// returning the original error.
			it.RegisterImportFailure(err)
			if _, nerr := t.tagcli.TaggerV1beta1().Tags(it.Namespace).UpdateStatus(
				ctx, it, metav1.UpdateOptions{},
			); nerr != nil {
				klog.Errorf("error updating tag status: %s", nerr)
			}
			metrics.ImportFailures.Inc()
			return fmt.Errorf("fail importing %s/%s: %w", it.Namespace, it.Name, err)
		}
		it.RegisterImportSuccess()
		it.PrependHashReference(hashref)
		metrics.ImportSuccesses.Inc()

		klog.Infof("tag %s/%s imported.", it.Namespace, it.Name)
	}

	genmatch := it.Spec.Generation == it.Status.Generation
	if alreadyImported && genmatch {
		return nil
	}

	it.Status.Generation = it.Spec.Generation
	if _, err = t.tagcli.TaggerV1beta1().Tags(it.Namespace).UpdateStatus(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return fmt.Errorf("error updating tag: %w", err)
	}
	return nil
}

// NewGenerationForImageRef looks through all image tags we have and creates a
// new generation in all those who point to the provided image path. Image path
// is a string that looks like "quay.io/repo/image:tag". XXX This function does
// not consider unqualified registries.
func (t *Tag) NewGenerationForImageRef(ctx context.Context, imgpath string) error {
	tags, err := t.taglis.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("fail to list tags: %w", err)
	}

	for _, tag := range tags {
		if tag.Spec.From != imgpath {
			continue
		}

		// tag has not been imported yet, it makes no sense to create
		// a new generation for it.
		if len(tag.Status.References) == 0 {
			continue
		}

		if !tag.SpecTagImported() {
			continue
		}

		tag.Spec.Generation = tag.NextGeneration()
		if _, err := t.tagcli.TaggerV1beta1().Tags(tag.Namespace).Update(
			ctx, tag, metav1.UpdateOptions{},
		); err != nil {
			return fmt.Errorf("fail updating tag: %w", err)
		}
	}

	return nil
}

// Upgrade increments the expected (spec) generation for a tag.
func (t *Tag) Upgrade(ctx context.Context, ns, name string) (*imagtagv1beta1.Tag, error) {
	it, err := t.tagcli.TaggerV1beta1().Tags(ns).Get(
		ctx, name, metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("fail to get tag: %w", err)
	}

	if !it.SpecTagImported() {
		return nil, fmt.Errorf("tag not imported yet")
	}

	// we only go as far as setting spec to the newest generation.
	it.Spec.Generation++
	if !it.SpecTagImported() {
		return nil, fmt.Errorf("currently at newest generation")
	}

	if it, err = t.tagcli.TaggerV1beta1().Tags(ns).Update(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return nil, fmt.Errorf("error updating tag: %w", err)
	}

	return it, nil
}

// Downgrade decrements the expected (spec) generation for a tag.
func (t *Tag) Downgrade(ctx context.Context, ns, name string) (*imagtagv1beta1.Tag, error) {
	it, err := t.tagcli.TaggerV1beta1().Tags(ns).Get(
		ctx, name, metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("error getting tag: %w", err)
	}

	it.Spec.Generation--
	if !it.SpecTagImported() {
		return nil, fmt.Errorf("currently at oldest generation")
	}

	if it, err = t.tagcli.TaggerV1beta1().Tags(ns).Update(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return nil, fmt.Errorf("error updating tag: %w", err)
	}
	return it, nil
}

// NewGeneration creates a new generation for a tag. The new generation is set
// to 'last imported generation + 1'. If no generation was imported then the next
// generation is zero.
func (t *Tag) NewGeneration(ctx context.Context, ns, name string) (*imagtagv1beta1.Tag, error) {
	it, err := t.tagcli.TaggerV1beta1().Tags(ns).Get(
		ctx, name, metav1.GetOptions{},
	)
	if err != nil {
		return nil, err
	}

	nextGen := int64(0)
	if len(it.Status.References) > 0 {
		nextGen = it.Status.References[0].Generation + 1
	}
	it.Spec.Generation = nextGen

	if it, err = t.tagcli.TaggerV1beta1().Tags(ns).Update(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return nil, fmt.Errorf("error updating tag: %w", err)
	}
	return it, nil
}

// Get returns a Tag object. Returned object is already a copy of the cached
// object and may be modified by caller as needed.
func (t *Tag) Get(ctx context.Context, ns, name string) (*imagtagv1beta1.Tag, error) {
	tag, err := t.taglis.Tags(ns).Get(name)
	if err != nil {
		return nil, err
	}
	return tag.DeepCopy(), nil
}

// AddEventHandler adds a handler to Tag related events.
func (t *Tag) AddEventHandler(handler cache.ResourceEventHandler) {
	t.taginf.Tagger().V1beta1().Tags().Informer().AddEventHandler(handler)
}

// splitRegistryDomain splits the domain from the repository and image.
// For example passing in the "quay.io/tagger/tagger:latest" string will
// result in "quay.io" and "tagger/tagger:latest".
func (t *Tag) splitRegistryDomain(imgPath string) (string, string) {
	imageSlices := strings.SplitN(imgPath, "/", 2)
	if len(imageSlices) < 2 {
		return "", imgPath
	}

	// if domain does not contain ".", ":" and is not "localhost"
	// we don't consider it a domain at all, return empty.
	if !strings.ContainsAny(imageSlices[0], ".:") && imageSlices[0] != "localhost" {
		return "", imgPath
	}

	return imageSlices[0], imageSlices[1]
}

// ImportTag runs an import on provided Tag. By Import here we mean to discover
// what is the current hash for a given image in a given tag. We look for the image
// in all configured unqualified registries using all authentications we can find
// for the registry in the Tag namespace. If the tag is set to be mirrored we push
// the image to our mirror registry.
func (t *Tag) ImportTag(
	ctx context.Context, it *imagtagv1beta1.Tag,
) (imagtagv1beta1.HashReference, error) {
	var zero imagtagv1beta1.HashReference
	if it.Spec.From == "" {
		return zero, fmt.Errorf("empty tag reference")
	}
	domain, remainder := t.splitRegistryDomain(it.Spec.From)

	registries, err := t.syssvc.RegistriesToSearch(ctx, domain)
	if err != nil {
		return zero, fmt.Errorf("fail to find source image domain: %w", err)
	}

	var errors *multierror.Error
	for _, registry := range registries {
		imgpath := fmt.Sprintf("docker://%s/%s", registry, remainder)
		imgref, err := alltransports.ParseImageName(imgpath)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		sysctxs, err := t.syssvc.SystemContextsFor(ctx, imgref, it.Namespace)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		imghash, sysctx, err := t.HashReferenceByTag(ctx, imgref, sysctxs)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		if it.Spec.Mirror {
			istore, err := t.syssvc.GetRegistryStore(ctx)
			if err != nil {
				return zero, fmt.Errorf("unable to get image store: %w", err)
			}

			mstart := time.Now()
			if imghash, err = istore.Load(
				ctx, imghash, sysctx, it.Namespace, it.Name,
			); err != nil {
				return zero, fmt.Errorf("fail to mirror image: %w", err)
			}
			metrics.MirrorLatency.Observe(time.Since(mstart).Seconds())
		}

		return imagtagv1beta1.HashReference{
			Generation:     it.Spec.Generation,
			From:           it.Spec.From,
			ImportedAt:     metav1.NewTime(time.Now()),
			ImageReference: imghash.DockerReference().String(),
		}, nil
	}

	return zero, fmt.Errorf("unable to import image: %w", errors)
}

// getImageHash attempts to fetch image hash remotely using provided system context.
// Hash is full image path with its hash, something like reg.io/repo/img@sha256:...
// The ideia here is that the "from" reference points to a image by tag, something
// like reg.io/repo/img:latest.
func (t *Tag) getImageHash(
	ctx context.Context, from types.ImageReference, sysctx *types.SystemContext,
) (types.ImageReference, error) {
	img, err := from.NewImage(ctx, sysctx)
	if err != nil {
		return nil, fmt.Errorf("unable to create image closer: %w", err)
	}
	defer img.Close()

	manifestBlob, _, err := img.Manifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch image manifest: %w", err)
	}

	dgst, err := manifest.Digest(manifestBlob)
	if err != nil {
		return nil, fmt.Errorf("error calculating manifest digest: %w", err)
	}

	refstr := fmt.Sprintf("docker://%s@%s", from.DockerReference().Name(), dgst)
	hashref, err := alltransports.ParseImageName(refstr)
	if err != nil {
		return nil, err
	}
	return hashref, nil
}

// HashReferenceByTag attempts to obtain the hash for a given image on a remote registry.
// It receives an image reference pointing to an image by its tag (reg.io/repo/img:tag)
// and returns a image reference by hash (reg.io/repo/img@sha256:abc...). It runs through
// provided system contexts trying all of them. If no SystemContext is present it does one
// attempt without authentication. Returns the image reference and the SystemContext that
// worked or an error.
func (t *Tag) HashReferenceByTag(
	ctx context.Context, imgref types.ImageReference, sysctxs []*types.SystemContext,
) (types.ImageReference, *types.SystemContext, error) {
	// if no contexts then we do an attempt without using any credentials.
	if len(sysctxs) == 0 {
		sysctxs = []*types.SystemContext{nil}
	}

	var errors *multierror.Error
	for _, sysctx := range sysctxs {
		imghash, err := t.getImageHash(ctx, imgref, sysctx)
		if err == nil {
			return imghash, sysctx, nil
		}
		errors = multierror.Append(errors, err)
	}
	return nil, nil, fmt.Errorf("unable to get hash for image tag: %w", errors)
}

// NewTag creates and saves a new Tag object.
func (t *Tag) NewTag(ctx context.Context, namespace, name, from string, mirror bool) error {
	it := &imagtagv1beta1.Tag{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: imagtagv1beta1.TagSpec{
			From:   from,
			Mirror: mirror,
		},
	}
	_, err := t.tagcli.TaggerV1beta1().Tags(namespace).Create(
		ctx, it, metav1.CreateOptions{},
	)
	return err
}

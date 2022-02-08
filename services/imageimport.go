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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"

	imgv1b1 "github.com/ricardomaraschini/tagger/infra/images/v1beta1"
	imgclient "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/clientset/versioned"
	imginform "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/informers/externalversions"
	imglist "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/listers/images/v1beta1"
	"github.com/ricardomaraschini/tagger/infra/metrics"
)

// ImageImport gather all actions related to image import objects.
type ImageImport struct {
	imgcli imgclient.Interface
	imglis imglist.ImageLister
	implis imglist.ImageImportLister
	imginf imginform.SharedInformerFactory
	syssvc *SysContext
}

// NewImageImport returns a handler for all Image import related services. I have chosen to go
// with a lazy approach here, you can pass or omit (nil) any parameter, it is up to the caller
// to decide what is needed for each specific case. So far this is the best approach, I still
// plan to review this.
func NewImageImport(
	corinf informers.SharedInformerFactory,
	imgcli imgclient.Interface,
	imginf imginform.SharedInformerFactory,
) *ImageImport {
	var implis imglist.ImageImportLister
	var imglis imglist.ImageLister
	if imginf != nil {
		implis = imginf.Tagger().V1beta1().ImageImports().Lister()
		imglis = imginf.Tagger().V1beta1().Images().Lister()
	}

	return &ImageImport{
		imginf: imginf,
		imgcli: imgcli,
		implis: implis,
		imglis: imglis,
		syssvc: NewSysContext(corinf),
	}
}

// ImportOpts holds the options necessary to call ImageImport.NewImport().
type ImportOpts struct {
	Namespace   string
	TargetImage string
	From        string
	Mirror      *bool
	Insecure    *bool
}

// NewImport uses provided ImportOpts to create a new ImageImport object and send it to the
// cluster. Returns the created object or an error.
func (t *ImageImport) NewImport(ctx context.Context, o ImportOpts) (*imgv1b1.ImageImport, error) {
	impid := strings.ReplaceAll(uuid.New().String(), "-", "")
	impid = impid[0:8]

	ii := &imgv1b1.ImageImport{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: o.Namespace,
			Name:      fmt.Sprintf("%s-%s", o.TargetImage, impid),
		},
		Spec: imgv1b1.ImageImportSpec{
			TargetImage: o.TargetImage,
			From:        o.From,
			Mirror:      o.Mirror,
			Insecure:    o.Insecure,
		},
	}

	return t.imgcli.TaggerV1beta1().ImageImports(o.Namespace).Create(
		ctx, ii, metav1.CreateOptions{},
	)
}

// NewImageFor creates a new Image object based on provided ImageImport. Embrace yourselves, from
// now on I declare WAR on this source code! XXX it may be a good idea to merge ImageImport and
// Image services into a single entity.
func (t *ImageImport) NewImageFor(
	ctx context.Context, ii *imgv1b1.ImageImport,
) (*imgv1b1.Image, error) {
	opts := NewImageOpts{
		Namespace: ii.Namespace,
		Name:      ii.Spec.TargetImage,
		From:      ii.Spec.From,
		Mirror:    pointer.BoolDeref(ii.Spec.Mirror, false),
		Insecure:  pointer.BoolDeref(ii.Spec.Insecure, false),
	}
	imgsvc := NewImage(nil, t.imgcli, nil)
	return imgsvc.NewImage(ctx, opts)
}

// Delete deletes an ImageImport according to some rules. In order to delete an import this
// import must be flagged as consumed for at least one hour. The exception made is if the
// import has a bogus or "unparseable" consume timestamp, then we log the fact and delete.
// We only return an error when we actually attempt to delete using k8s api, if the import
// is filtered out by any of the forementioned rules a nil is returned instead.
func (t *ImageImport) Delete(ctx context.Context, ii *imgv1b1.ImageImport) error {
	if !ii.FlaggedAsConsumed() {
		return nil
	}

	// we avoid to delete ImageImport whose consumed flag is readable and that have been
	// flagged as consumed less than one hour ago.
	duration, err := ii.FlaggedAsConsumedDuration()
	if err == nil && duration < time.Hour {
		return nil
	}

	// if we could not parse the consume flag then we at least log this fact. We gonna go
	// ahead and delete the ImageImport.
	if err != nil {
		klog.Infof("deleting %s/%s: %s", ii.Namespace, ii.Name, err)
	}

	return t.imgcli.TaggerV1beta1().ImageImports(ii.Namespace).Delete(
		ctx, ii.Name, metav1.DeleteOptions{},
	)
}

// Sync manages image import change, assuring we have the image imported. Beware that we change
// ImageImport in place before updating it on api server, i.e. use DeepCopy() before passing the
// image import in.
func (t *ImageImport) Sync(ctx context.Context, ii *imgv1b1.ImageImport) error {
	if err := ii.Validate(); err != nil {
		return fmt.Errorf("invalid image import: %w", err)
	}

	if ii.FlaggedAsConsumed() {
		if err := t.Delete(ctx, ii); err != nil {
			klog.V(5).Infof(
				"unable to delete image import %s/%s: %s",
				ii.Namespace, ii.Name, err,
			)
		}
		return nil
	}

	if ii.AlreadyImported() {
		klog.Infof("image import %s/%s already executed", ii.Namespace, ii.Name)
		return nil
	}

	// if no more attempts are going to be made on this ImageImport we can flag it for
	// deletion. Deletion tends to take a while, check Delete() func for more on this.
	if ii.FailedImportAttempts() >= imgv1b1.MaxImportAttempts {
		ii.FlagAsConsumed()
		if _, err := t.imgcli.TaggerV1beta1().ImageImports(ii.Namespace).Update(
			ctx, ii, metav1.UpdateOptions{},
		); err != nil {
			klog.V(5).Infof(
				"unable to flag image import %s/%s for deletion: %s",
				ii.Namespace, ii.Name, err,
			)
		}

		klog.Infof("image import %s/%s has failed", ii.Namespace, ii.Name)
		return nil
	}

	klog.Infof("image import %s/%s needs import, importing...", ii.Namespace, ii.Name)
	img, err := t.imgcli.TaggerV1beta1().Images(ii.Namespace).Get(
		ctx, ii.Spec.TargetImage, metav1.GetOptions{},
	)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("unable to get target image: %w", err)
		}

		// We will create a new Image if none exist.
		if img, err = t.NewImageFor(ctx, ii); err != nil {
			return fmt.Errorf("unable to create target image: %w", err)
		}

		klog.Infof("new image %s/%s created", img.Namespace, img.Name)
	}

	if !ii.OwnedByImage(img) {
		ii.SetOwnerImage(img)
		if ii, err = t.imgcli.TaggerV1beta1().ImageImports(ii.Namespace).Update(
			ctx, ii, metav1.UpdateOptions{},
		); err != nil {
			klog.Errorf("error setting image import owner: %s", err)
			return fmt.Errorf("error processing image import: %w", err)
		}
	}

	// make sure we inherited values from the target Image object. This essentially means
	// that we must have no nil pointers in the ImageImport object.
	ii.InheritValuesFrom(img)
	if ii.Spec.From == "" {
		return fmt.Errorf("unable to determine image source registry")
	}

	hashref, err := t.Import(ctx, ii)
	if err != nil {
		metrics.ImportFailures.Inc()
		ii.RegisterImportFailure(err)
		if _, nerr := t.imgcli.TaggerV1beta1().ImageImports(ii.Namespace).UpdateStatus(
			ctx, ii, metav1.UpdateOptions{},
		); nerr != nil {
			klog.Errorf("error updating image import status: %s", nerr)
		}
		return fmt.Errorf("fail importing %s/%s: %w", ii.Namespace, ii.Name, err)
	}

	ii.RegisterImportSuccess()
	ii.Status.HashReference = hashref
	if _, err = t.imgcli.TaggerV1beta1().ImageImports(ii.Namespace).UpdateStatus(
		ctx, ii, metav1.UpdateOptions{},
	); err != nil {
		return fmt.Errorf("error updating image import: %w", err)
	}

	metrics.ImportSuccesses.Inc()
	klog.Infof("image import %s/%s processed.", ii.Namespace, ii.Name)
	return nil
}

// Import runs an import on provided ImageImport. By Import here we mean to discover
// what is the current hash for a given image in a given tag. We look for the image
// in all configured unqualified registries using all authentications we can find
// for the registry in the ImageImport namespace. If the image is set to be mirrored
// we push the image to our mirror registry.
func (t *ImageImport) Import(
	ctx context.Context, ii *imgv1b1.ImageImport,
) (*imgv1b1.HashReference, error) {
	domain, remainder := t.splitRegistryDomain(ii.Spec.From)

	registries, err := t.syssvc.RegistriesToSearch(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("fail to find source image domain: %w", err)
	}

	var errors *multierror.Error
	for _, registry := range registries {
		imgpath := fmt.Sprintf("docker://%s/%s", registry, remainder)
		imgref, err := alltransports.ParseImageName(imgpath)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		insecure := pointer.BoolDeref(ii.Spec.Insecure, false)
		sysctxs, err := t.syssvc.SystemContextsFor(ctx, imgref, ii.Namespace, insecure)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		imghash, sysctx, err := t.HashReferenceByImage(ctx, imgref, sysctxs)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		if mirror := pointer.BoolDeref(ii.Spec.Mirror, false); mirror {
			istore, err := t.syssvc.GetRegistryStore(ctx)
			if err != nil {
				return nil, fmt.Errorf("unable to get image store: %w", err)
			}

			start := time.Now()
			timg := ii.Spec.TargetImage
			imghash, err = istore.Load(ctx, imghash, sysctx, ii.Namespace, timg)
			if err != nil {
				return nil, fmt.Errorf("fail to mirror image: %w", err)
			}

			latency := time.Now().Sub(start).Seconds()
			metrics.MirrorLatency.Observe(latency)
		}

		return &imgv1b1.HashReference{
			From:           ii.Spec.From,
			ImportedAt:     metav1.NewTime(time.Now()),
			ImageReference: imghash.DockerReference().String(),
		}, nil
	}

	return nil, fmt.Errorf("unable to import image: %w", errors)
}

// splitRegistryDomain splits the domain from the repository and image.  For example passing in
// the "quay.io/tagger/tagger:latest" string will result in "quay.io" and "tagger/tagger:latest".
func (t *ImageImport) splitRegistryDomain(imgPath string) (string, string) {
	imageSlices := strings.SplitN(imgPath, "/", 2)
	if len(imageSlices) < 2 {
		return "", imgPath
	}

	// if domain does not contain ".", ":" and is not "localhost" we don't consider it a
	// domain at all, return empty.
	if !strings.ContainsAny(imageSlices[0], ".:") && imageSlices[0] != "localhost" {
		return "", imgPath
	}

	return imageSlices[0], imageSlices[1]
}

// Get returns a ImageImport object. Returned object is already a copy of the cached object and
// may be modified by caller as needed.
func (t *ImageImport) Get(ctx context.Context, ns, name string) (*imgv1b1.ImageImport, error) {
	imp, err := t.implis.ImageImports(ns).Get(name)
	if err != nil {
		return nil, fmt.Errorf("unable to get image import: %w", err)
	}
	return imp.DeepCopy(), nil
}

// Validate checks if provided ImageImport contain all mandatory fields. If ImageImport does
// contains an empty "spec.from" we attempt to load the targetImage.
func (t *ImageImport) Validate(ctx context.Context, imp *imgv1b1.ImageImport) error {
	if err := imp.Validate(); err != nil {
		return err
	}

	if _, err := t.imglis.Images(imp.Namespace).Get(imp.Spec.TargetImage); err != nil {
		if !errors.IsNotFound(err) {
			return err
		} else if imp.Spec.From == "" {
			return fmt.Errorf("empty spec.from")
		}
	}
	return nil
}

// AddEventHandler adds a handler to Image related events.
func (t *ImageImport) AddEventHandler(handler cache.ResourceEventHandler) {
	t.imginf.Tagger().V1beta1().ImageImports().Informer().AddEventHandler(handler)
}

// HashReferenceByImage attempts to obtain the hash for a given image on a remote registry.
// It receives an image reference pointing to an image by its tag (reg.io/repo/img:tag)
// and returns a image reference by hash (reg.io/repo/img@sha256:abc...). It runs through
// provided system contexts trying all of them. If no SystemContext is present it does one
// attempt without authentication. Returns the image reference and the SystemContext that
// worked or an error.
func (t *ImageImport) HashReferenceByImage(
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
	return nil, nil, fmt.Errorf("unable to get hash for image image: %w", errors)
}

// getImageHash attempts to fetch image hash remotely using provided system context. Hash is
// full image path with its hash, something like reg.io/repo/img@sha256:... The ideia here is
// that the "from" reference points to a image by tag, something like reg.io/repo/img:latest
// and we return a reference by hash (something like reg.io/repo/img@sha256:...).
func (t *ImageImport) getImageHash(
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

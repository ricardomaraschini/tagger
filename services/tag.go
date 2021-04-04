package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/hashicorp/go-multierror"
	"github.com/mattbaird/jsonpatch"

	imagtagv1 "github.com/ricardomaraschini/tagger/infra/tags/v1"
	tagclient "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/clientset/versioned"
	taginform "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/informers/externalversions"
	taglist "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/listers/tags/v1"
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
		taglis = taginf.Images().V1().Tags().Lister()
	}

	return &Tag{
		taginf: taginf,
		tagcli: tagcli,
		taglis: taglis,
		syssvc: NewSysContext(corinf),
	}
}

// PatchForPod creates and returns a json patch to be applied on top of a pod
// in order to make it point to an already imported image tag. May returns nil
// if no patch is needed (i.e. pod does not use image tag).
func (t *Tag) PatchForPod(pod corev1.Pod) ([]jsonpatch.JsonPatchOperation, error) {
	if _, ok := pod.Annotations["image-tag"]; !ok {
		return nil, nil
	}

	// TODO We need to check other types of containers within a pod. Here
	// we are going only for the containers on spec.containers.
	nconts := []corev1.Container{}
	for _, c := range pod.Spec.Containers {
		if ref, ok := pod.Annotations[c.Image]; ok {
			c.Image = ref
		}
		nconts = append(nconts, c)
	}
	changed := pod.DeepCopy()
	changed.Spec.Containers = nconts

	origData, err := json.Marshal(pod)
	if err != nil {
		return nil, fmt.Errorf("error marshaling original pod: %w", err)
	}
	changedData, err := json.Marshal(changed)
	if err != nil {
		return nil, fmt.Errorf("error marshaling updated pod: %w", err)
	}

	patch, err := jsonpatch.CreatePatch(origData, changedData)
	if err != nil {
		return nil, fmt.Errorf("fail creating patch for pod: %w", err)
	}

	// make sure we always return the zero value for a slice and not
	// an empty one.
	if len(patch) == 0 {
		return nil, nil
	}
	return patch, nil
}

// Sync manages image tag updates, assuring we have the tag imported.
// Beware that we change Tag in place before updating it on api server,
// i.e. use DeepCopy() before passing the image tag in.
func (t *Tag) Sync(ctx context.Context, it *imagtagv1.Tag) error {
	var err error
	var hashref imagtagv1.HashReference

	alreadyImported := it.SpecTagImported()
	if !alreadyImported {
		klog.Infof("tag %s/%s needs import, importing...", it.Namespace, it.Name)

		hashref, err = t.ImportTag(ctx, it)
		if err != nil {
			// if we fail to import the tag we need to record the failure on tag's
			// status and update it. If we fail to update the tag we only log,
			// returning the original error.
			it.RegisterImportFailure(err)
			if _, err := t.tagcli.ImagesV1().Tags(it.Namespace).Update(
				ctx, it, metav1.UpdateOptions{},
			); err != nil {
				klog.Errorf("error updating tag status: %s", err)
			}
			return fmt.Errorf("fail importing %s/%s: %w", it.Namespace, it.Name, err)
		}
		it.RegisterImportSuccess()
		it.PrependHashReference(hashref)

		klog.Infof("tag %s/%s imported.", it.Namespace, it.Name)
	}

	genmatch := it.Spec.Generation == it.Status.Generation
	if alreadyImported && genmatch {
		return nil
	}

	it.Status.Generation = it.Spec.Generation
	if _, err = t.tagcli.ImagesV1().Tags(it.Namespace).Update(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return fmt.Errorf("error updating tag: %w", err)
	}
	return nil
}

// NewGenerationForImageRef looks through all image tags we have and creates a
// new generation in all of those who point to the provided image path. Image
// path looks like "quay.io/repo/image:tag".
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
			// we still have a pending import for this image
			continue
		}

		tag.Spec.Generation = tag.Status.References[0].Generation + 1
		if _, err := t.tagcli.ImagesV1().Tags(tag.Namespace).Update(
			ctx, tag, metav1.UpdateOptions{},
		); err != nil {
			return fmt.Errorf("fail updating tag: %w", err)
		}
	}

	return nil
}

// Upgrade increments the expected (spec) generation for a tag.
func (t *Tag) Upgrade(ctx context.Context, ns, name string) (*imagtagv1.Tag, error) {
	it, err := t.tagcli.ImagesV1().Tags(ns).Get(
		ctx, name, metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("fail to get tag: %w", err)
	}

	if !it.SpecTagImported() {
		return nil, fmt.Errorf("pending tag import")
	}

	it.Spec.Generation++
	if it, err = t.tagcli.ImagesV1().Tags(ns).Update(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return nil, fmt.Errorf("error updating tag: %w", err)
	}

	return it, nil
}

// Downgrade decrements the expected (spec) generation for a tag.
func (t *Tag) Downgrade(ctx context.Context, ns, name string) (*imagtagv1.Tag, error) {
	it, err := t.tagcli.ImagesV1().Tags(ns).Get(
		ctx, name, metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("error getting tag: %w", err)
	}

	it.Spec.Generation--
	if !it.SpecTagImported() {
		return nil, fmt.Errorf("unable to downgrade, currently at oldest generation")
	}

	if it, err = t.tagcli.ImagesV1().Tags(ns).Update(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return nil, fmt.Errorf("error updating tag: %w", err)
	}
	return it, nil
}

// NewGeneration creates a new generation for a tag. The new generation is set
// to 'last import generation + 1'. If no generation was imported then the next
// generation is zero.
func (t *Tag) NewGeneration(ctx context.Context, ns, name string) (*imagtagv1.Tag, error) {
	it, err := t.tagcli.ImagesV1().Tags(ns).Get(
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

	if it, err = t.tagcli.ImagesV1().Tags(ns).Update(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return nil, fmt.Errorf("error updating tag: %w", err)
	}
	return it, nil
}

// Get returns a Tag object. Returned object is already a copy of the cached
// object and may be modified by caller as needed.
func (t *Tag) Get(ctx context.Context, ns, name string) (*imagtagv1.Tag, error) {
	tag, err := t.taglis.Tags(ns).Get(name)
	if err != nil {
		return nil, fmt.Errorf("unable to get tag: %w", err)
	}
	return tag.DeepCopy(), nil
}

// AddEventHandler adds a handler to Tag related events.
func (t *Tag) AddEventHandler(handler cache.ResourceEventHandler) {
	t.taginf.Images().V1().Tags().Informer().AddEventHandler(handler)
}

// splitRegistryDomain splits the domain from the repository and image.
// For example passing in the "quay.io/tagger/tagger:latest" string will
// result in returned values "quay.io" and "tagger/tagger:latest".
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
	ctx context.Context, it *imagtagv1.Tag,
) (imagtagv1.HashReference, error) {
	var zero imagtagv1.HashReference
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

			if imghash, err = istore.Load(
				ctx, imghash, sysctx, it.Namespace, it.Name,
			); err != nil {
				return zero, fmt.Errorf("fail to mirror image: %w", err)
			}
		}

		return imagtagv1.HashReference{
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

// NewTag creates a new Tag objects.
func (t *Tag) NewTag(
	ctx context.Context, namespace, name, from string, mirror bool,
) error {
	it := &imagtagv1.Tag{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: imagtagv1.TagSpec{
			From:   from,
			Mirror: mirror,
		},
	}
	_, err := t.tagcli.ImagesV1().Tags(namespace).Create(
		ctx, it, metav1.CreateOptions{},
	)
	return err
}

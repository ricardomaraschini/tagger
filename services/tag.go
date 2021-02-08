package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	corecli "k8s.io/client-go/kubernetes"
	aplist "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/mattbaird/jsonpatch"

	tagclient "github.com/ricardomaraschini/tagger/imagetags/generated/clientset/versioned"
	taginform "github.com/ricardomaraschini/tagger/imagetags/generated/informers/externalversions"
	taglist "github.com/ricardomaraschini/tagger/imagetags/generated/listers/imagetags/v1"
	imagtagv1 "github.com/ricardomaraschini/tagger/imagetags/v1"
)

// Tag gather all actions related to image tag objects.
type Tag struct {
	tmpdir string
	tagcli tagclient.Interface
	taglis taglist.TagLister
	taginf taginform.SharedInformerFactory
	deplis aplist.DeploymentLister
	impsvc *Importer
	depsvc *Deployment
	syssvc *SysContext
	fstsvc *FS
}

// NewTag returns a handler for all image tag related services. I have chosen to
// go with a lazy approach here, you can pass or omit (nil) any parameter, it is
// up to the caller to decide what is needed for each specific case. So far this
// is the best approach, I still plan to review this.
func NewTag(
	corcli corecli.Interface,
	corinf informers.SharedInformerFactory,
	tagcli tagclient.Interface,
	taginf taginform.SharedInformerFactory,
) *Tag {
	var deplis aplist.DeploymentLister
	var taglis taglist.TagLister

	if corinf != nil {
		deplis = corinf.Apps().V1().Deployments().Lister()
	}

	if taginf != nil {
		taglis = taginf.Images().V1().Tags().Lister()
	}

	return &Tag{
		tmpdir: "/data",
		taginf: taginf,
		tagcli: tagcli,
		taglis: taglis,
		deplis: deplis,
		impsvc: NewImporter(corinf),
		depsvc: NewDeployment(corcli, corinf, taginf),
		fstsvc: NewFS(),
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

		hashref, err = t.impsvc.ImportTag(ctx, it)
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

	genMismatch := it.Spec.Generation != it.Status.Generation
	if !alreadyImported || genMismatch {
		it.Status.Generation = it.Spec.Generation
		if it, err = t.tagcli.ImagesV1().Tags(it.Namespace).Update(
			ctx, it, metav1.UpdateOptions{},
		); err != nil {
			return fmt.Errorf("error updating image stream: %w", err)
		}
	}

	return t.depsvc.UpdateDeploymentsForTag(ctx, it)
}

// NewGenerationForImageRef looks through all image tags we have and creates a
// new generation in all of those who point to the provided image path. Image
// path looks like "quay.io/repo/image:tag". TODO add unqualified registries
// support and consider also empty tag as "latest".
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

// Upgrade increments the expected (spec) generation for a tag. This function updates
// the object through the kubernetes api.
func (t *Tag) Upgrade(
	ctx context.Context, namespace string, name string,
) (*imagtagv1.Tag, error) {
	it, err := t.tagcli.ImagesV1().Tags(namespace).Get(
		ctx, name, metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("fail to get tag: %w", err)
	}

	if !it.SpecTagImported() {
		return nil, fmt.Errorf("pending tag import")
	}

	it.Spec.Generation++
	if it, err = t.tagcli.ImagesV1().Tags(namespace).Update(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return nil, fmt.Errorf("error updating tag: %w", err)
	}

	return it, nil
}

// Downgrade increments the expected (spec) generation for a tag. This function
// updates the object through the kubernetes api.
func (t *Tag) Downgrade(
	ctx context.Context, namespace string, name string,
) (*imagtagv1.Tag, error) {
	it, err := t.tagcli.ImagesV1().Tags(namespace).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("error getting tag: %w", err)
	}

	it.Spec.Generation--
	if !it.SpecTagImported() {
		return nil, fmt.Errorf("unable to downgrade, currently at oldest generation")
	}

	if it, err = t.tagcli.ImagesV1().Tags(namespace).Update(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return nil, fmt.Errorf("error updating tag: %w", err)
	}
	return it, nil
}

// NewGeneration creates a new generation for a tag. The new generation is set
// to 'last import generation + 1'. If no generation was imported then the next
// generation is zero.
func (t *Tag) NewGeneration(
	ctx context.Context, namespace string, name string,
) (*imagtagv1.Tag, error) {
	it, err := t.tagcli.ImagesV1().Tags(namespace).Get(
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

	if it, err = t.tagcli.ImagesV1().Tags(namespace).Update(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return nil, fmt.Errorf("error updating tag: %w", err)
	}
	return it, nil
}

// Export saves a Tag into a local tar file and returns a reader closer to
// it.  Caller is responsible for cleaning up after the returned value.
func (t *Tag) Export(
	ctx context.Context, ns string, name string,
) (io.ReadCloser, error) {
	it, err := t.taglis.Tags(ns).Get(name)
	if err != nil {
		return nil, fmt.Errorf("error getting tag: %w", err)
	}

	dir, err := ioutil.TempDir(t.tmpdir, "tag-export-*")
	if err != nil {
		return nil, fmt.Errorf("error creating temp dir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			klog.Errorf("error removing temp dir: %s", err)
		}
	}()

	if err := t.impsvc.PullTagToDir(ctx, it, dir); err != nil {
		return nil, fmt.Errorf("error pulling tag to dir: %w", err)
	}

	if err := t.cleanAndEncodeTag(it, dir); err != nil {
		return nil, fmt.Errorf("error encoding tag: %w", err)
	}

	out, err := ioutil.TempFile(t.tmpdir, "tar-export-*.tar.gz")
	if err != nil {
		return nil, fmt.Errorf("error creating tar file: %w", err)
	}

	if err := t.fstsvc.CompressDirectory(dir, out); err != nil {
		out.Close()
		if err := os.Remove(out.Name()); err != nil {
			klog.Errorf("error removing temp tar: %s", err)
		}
		return nil, fmt.Errorf("error compressing tag: %w", err)
	}
	return out, nil
}

// cleanAndEncodeTag json encodes the Tag referred by TagExport and stores it in
// a file callled tag.json inside the provided directory.
func (t *Tag) cleanAndEncodeTag(it *imagtagv1.Tag, dir string) error {
	tcopy, err := t.cleanTag(it)
	if err != nil {
		return fmt.Errorf("unable to clear tag: %w", err)
	}

	tfpath := fmt.Sprintf("%s/tag.json", dir)
	tf, err := os.Create(tfpath)
	if err != nil {
		return fmt.Errorf("error creating file for encoded tag: %w", err)
	}
	defer tf.Close()

	return json.NewEncoder(tf).Encode(tcopy)
}

// cleanTag changes all references to the cache registry by a template entry so it
// can be reassembled later on in another cluster. Cleans up the tag namespace as
// well.
func (t *Tag) cleanTag(it *imagtagv1.Tag) (*imagtagv1.Tag, error) {
	inregaddr, _, err := t.syssvc.CacheRegistryAddresses()
	if err != nil {
		return nil, fmt.Errorf("error cleaning up tag: %w", err)
	}

	it = it.DeepCopy()
	namespace := fmt.Sprintf("/%s/", it.Namespace)
	it.ObjectMeta = metav1.ObjectMeta{
		Name: it.Name,
	}
	for i := range it.Status.References {
		imgref := it.Status.References[i].ImageReference
		if !strings.HasPrefix(imgref, inregaddr) {
			continue
		}

		imgref = strings.ReplaceAll(imgref, inregaddr, "{{.Registry}}")
		imgref = strings.ReplaceAll(imgref, namespace, "/{{.Namespace}}/")
		it.Status.References[i].ImageReference = imgref
	}
	return it, nil
}

// Get returns a tag by namespace and name pair.
func (t *Tag) Get(ctx context.Context, ns, name string) (*imagtagv1.Tag, error) {
	tag, err := t.taglis.Tags(ns).Get(name)
	if err != nil {
		return nil, fmt.Errorf("unable to get tag: %w", err)
	}
	return tag.DeepCopy(), nil
}

// AddEventHandler adds a handler to tag related events.
func (t *Tag) AddEventHandler(handler cache.ResourceEventHandler) {
	t.taginf.Images().V1().Tags().Informer().AddEventHandler(handler)
}

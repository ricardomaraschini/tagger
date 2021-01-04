package services

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corecli "k8s.io/client-go/kubernetes"
	aplist "k8s.io/client-go/listers/apps/v1"
	corelister "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"

	"github.com/mattbaird/jsonpatch"

	tagclient "github.com/ricardomaraschini/tagger/imagetags/generated/clientset/versioned"
	taglist "github.com/ricardomaraschini/tagger/imagetags/generated/listers/imagetags/v1"
	imagtagv1 "github.com/ricardomaraschini/tagger/imagetags/v1"
)

// Tag gather all actions related to image tag objects.
type Tag struct {
	tagcli tagclient.Interface
	taglis taglist.TagLister
	replis aplist.ReplicaSetLister
	deplis aplist.DeploymentLister
	impsvc *Importer
	depsvc *Deployment
}

// NewTag returns a handler for all image tag related services.
func NewTag(
	corcli corecli.Interface,
	tagcli tagclient.Interface,
	taglis taglist.TagLister,
	replis aplist.ReplicaSetLister,
	deplis aplist.DeploymentLister,
	cmlister corelister.ConfigMapLister,
	sclister corelister.SecretLister,
) *Tag {
	return &Tag{
		tagcli: tagcli,
		taglis: taglis,
		replis: replis,
		deplis: deplis,
		impsvc: NewImporter(cmlister, sclister),
		depsvc: NewDeployment(corcli, deplis, taglis),
	}
}

// CurrentReferenceForTagByName returns the image reference a tag is pointing to.
// If we can't find the image tag by namespace and name an empty string is returned
// instead.
func (t *Tag) CurrentReferenceForTagByName(namespace, name string) (string, error) {
	it, err := t.taglis.Tags(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return it.CurrentReferenceForTag(), nil
}

// PatchForPod creates and returns a json patch to be applied on top of a pod
// in order to make it point to an already imported image tag. May returns nil
// if no patch is needed (i.e. pod does not use image tag).
func (t *Tag) PatchForPod(pod corev1.Pod) ([]jsonpatch.JsonPatchOperation, error) {
	if len(pod.OwnerReferences) == 0 {
		return nil, nil
	}

	// TODO multiple / different types of owners
	podOwner := pod.OwnerReferences[0]
	if podOwner.Kind != "ReplicaSet" {
		return nil, nil
	}

	rs, err := t.replis.ReplicaSets(pod.Namespace).Get(podOwner.Name)
	if err != nil {
		return nil, err
	}

	// if the replica set has no image tag annotation there is nothing to
	// be patched.
	if _, ok := rs.Annotations["image-tag"]; !ok {
		return nil, nil
	}

	// TODO We need to check other types of containers within a pod. Here
	// we are going only for the containers on spec.containers.
	nconts := []corev1.Container{}
	for _, c := range pod.Spec.Containers {
		ref, err := t.CurrentReferenceForTagByName(pod.Namespace, c.Image)
		if err != nil {
			return nil, err
		}

		if ref != "" {
			c.Image = ref
		}
		nconts = append(nconts, c)
	}
	changed := pod.DeepCopy()
	changed.Spec.Containers = nconts

	origData, err := json.Marshal(pod)
	if err != nil {
		return nil, err
	}
	changedData, err := json.Marshal(changed)
	if err != nil {
		return nil, err
	}

	patch, err := jsonpatch.CreatePatch(origData, changedData)
	if err != nil {
		return nil, err
	}

	// make sure we always return the zero value for a slice and not
	// an empty one.
	if len(patch) == 0 {
		return nil, nil
	}
	return patch, nil
}

// Update manages image tag updates, assuring we have the tag imported.
// Beware that we change Tag in place before updating it on api server,
// i.e. use DeepCopy() before passing the image tag in.
func (t *Tag) Update(ctx context.Context, it *imagtagv1.Tag) error {
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
			return fmt.Errorf("fail import %s/%s: %w", it.Namespace, it.Name, err)
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
		return err
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

		lastImport := tag.Status.References[0]
		if lastImport.Generation != tag.Spec.Generation {
			// we still have a pending import for this image
			continue
		}

		tag.Spec.Generation++
		if _, err := t.tagcli.ImagesV1().Tags(tag.Namespace).Update(
			ctx, tag, metav1.UpdateOptions{},
		); err != nil {
			return err
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
		return nil, err
	}

	it.Spec.Generation++

	return t.tagcli.ImagesV1().Tags(namespace).Update(
		ctx, it, metav1.UpdateOptions{},
	)
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
		return nil, err
	}

	it.Spec.Generation--
	if !it.SpecTagImported() {
		return nil, fmt.Errorf("unable to downgrade, currently at oldest generation")
	}

	return t.tagcli.ImagesV1().Tags(namespace).Update(
		context.Background(), it, metav1.UpdateOptions{},
	)
}

// NextGeneration creates a new generation for a tag. The new generation is set
// to 'last import generation + 1'. If no generation was imported then the next
// generation is zero.
func (t *Tag) NextGeneration(
	ctx context.Context, namespace string, name string,
) (*imagtagv1.Tag, error) {
	tag, err := t.tagcli.ImagesV1().Tags(namespace).Get(
		ctx, name, metav1.GetOptions{},
	)
	if err != nil {
		return nil, err
	}

	nextGen := int64(0)
	if len(tag.Status.References) > 0 {
		nextGen = tag.Status.References[0].Generation + 1
	}
	tag.Spec.Generation = nextGen

	return t.tagcli.ImagesV1().Tags(namespace).Update(
		ctx, tag, metav1.UpdateOptions{},
	)
}

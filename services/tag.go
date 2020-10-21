package services

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	aplist "k8s.io/client-go/listers/apps/v1"
	"k8s.io/cri-api/pkg/errors"
	"k8s.io/klog/v2"

	"github.com/mattbaird/jsonpatch"

	tagclient "github.com/ricardomaraschini/it/imagetags/generated/clientset/versioned"
	tagliss "github.com/ricardomaraschini/it/imagetags/generated/listers/imagetags/v1"
	imagtagv1 "github.com/ricardomaraschini/it/imagetags/v1"
)

// Tag gather all actions related to image tag objects.
type Tag struct {
	tagcli tagclient.Interface
	taglis tagliss.TagLister
	replis aplist.ReplicaSetLister
	impsvc *Importer
}

// NewTag returns a handler for all image tag related services.
func NewTag(
	tagcli tagclient.Interface,
	taglis tagliss.TagLister,
	replis aplist.ReplicaSetLister,
	impsvc *Importer,
) *Tag {
	return &Tag{
		tagcli: tagcli,
		taglis: taglis,
		replis: replis,
		impsvc: impsvc,
	}
}

// DockerReferenceForTag returns the docker reference for an image tag. If
// the image tag does not exist empty string is returned.
func (t *Tag) DockerReferenceForTag(namespace, name string) (string, error) {
	it, err := t.taglis.Tags(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	if !t.tagAlreadyImported(it) {
		return "", fmt.Errorf("tag needs import")
	}
	return it.Status.ImageReference, nil
}

// PatchForPod creates and returns a json patch to be applied on top of a pod
// in order to make it point to an already imported image tag. May returns nil
// if no patch is needed (i.e. pod does not use image tag).
func (t *Tag) PatchForPod(pod corev1.Pod) ([]byte, error) {
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

	// if the replica set has no image stream annotation there is
	// nothing to be patched.
	if _, ok := rs.Annotations["image-tag"]; !ok {
		return nil, nil
	}

	nconts := []corev1.Container{}
	for _, c := range pod.Spec.Containers {
		ref, err := t.DockerReferenceForTag(pod.Namespace, c.Image)
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
	chngData, err := json.Marshal(changed)
	if err != nil {
		return nil, err
	}

	patch, err := jsonpatch.CreatePatch(origData, chngData)
	if err != nil {
		return nil, err
	}
	return json.Marshal(patch)
}

// tagAlreadyImported returs true if tag has already been imported by
// comparing its spec with its status.
func (t *Tag) tagAlreadyImported(it *imagtagv1.Tag) bool {
	importedOnce := it.Status.Generation > 0
	sameGeneration := it.Status.Generation == it.Spec.Generation
	sameOrigin := it.Spec.From == it.Status.From
	return importedOnce && sameGeneration && sameOrigin
}

// Update manages image tag updates, assuring we have the tag imported.
// Beware that we change Tag in place before updating it on api server,
// i.e. use DeepCopy() before passing the image tag in.
func (t *Tag) Update(ctx context.Context, it *imagtagv1.Tag) error {
	if t.tagAlreadyImported(it) {
		klog.Infof("tag %s/%s already imported", it.Namespace, it.Name)
		return nil
	}

	klog.Infof("importing tag %s/%s", it.Namespace, it.Name)
	tagStatus, err := t.impsvc.ImportTag(ctx, it, it.Namespace)
	if err != nil {
		return fmt.Errorf("fail to import tag: %w", err)
	}
	it.Status = tagStatus

	if _, err := t.tagcli.ImagesV1().Tags(it.Namespace).Update(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return fmt.Errorf("error updating image stream: %w", err)
	}
	return nil
}

// Delete manages an image tag deletion. XXX we might need this function
// to clean up dangling objects associated with an tag but for now this
// is a no-op.
func (t *Tag) Delete(ctx context.Context, namespace, name string) error {
	return nil
}

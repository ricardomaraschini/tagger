package services

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corecli "k8s.io/client-go/kubernetes"
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
	corcli corecli.Interface
	tagcli tagclient.Interface
	taglis tagliss.TagLister
	replis aplist.ReplicaSetLister
	deplis aplist.DeploymentLister
	impsvc *Importer
}

// NewTag returns a handler for all image tag related services.
func NewTag(
	corcli corecli.Interface,
	tagcli tagclient.Interface,
	taglis tagliss.TagLister,
	replis aplist.ReplicaSetLister,
	deplis aplist.DeploymentLister,
	impsvc *Importer,
) *Tag {
	return &Tag{
		corcli: corcli,
		tagcli: tagcli,
		taglis: taglis,
		replis: replis,
		deplis: deplis,
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

// PatchForDeployment creates a patch to be applied on top of a deployment
// in order to keep track of what version of image tag it is using.
func (t *Tag) PatchForDeployment(deploy v1.Deployment) ([]byte, error) {
	if _, ok := deploy.Annotations["image-tag"]; !ok {
		return nil, nil
	}

	newAnnotations := map[string]string{}
	for _, c := range deploy.Spec.Template.Spec.Containers {
		is, err := t.taglis.Tags(deploy.Namespace).Get(c.Image)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return nil, err
		}

		if !t.tagAlreadyImported(is) {
			continue
		}

		newAnnotations[is.Name] = is.Status.LastUpdatedAt.String()
	}
	changed := deploy.DeepCopy()
	if changed.Spec.Template.Annotations == nil {
		changed.Spec.Template.Annotations = map[string]string{}
	}
	for key, val := range newAnnotations {
		changed.Spec.Template.Annotations[key] = val
	}

	origData, err := json.Marshal(deploy)
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

	// if the replica set has no image tag annotation there is nothing to
	// be patched.
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
		if err := t.updateDeployments(ctx, it); err != nil {
			return fmt.Errorf("unable to update deployments: %w", err)
		}
		return nil
	}

	klog.Infof("importing tag %s/%s", it.Namespace, it.Name)
	tagStatus, err := t.impsvc.ImportTag(ctx, it, it.Namespace)
	if err != nil {
		return fmt.Errorf("fail to import tag: %w", err)
	}
	it.Status = tagStatus

	if it, err = t.tagcli.ImagesV1().Tags(it.Namespace).Update(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return fmt.Errorf("error updating image stream: %w", err)
	}

	if err := t.updateDeployments(ctx, it); err != nil {
		return fmt.Errorf("unable to update deployments: %w", err)
	}
	return nil
}

// updateDeployments sets an annotation in all deployments using the provided
// image tag. It executes an idempotent operation with regards to annotation,
// i.e. it will trigger a deployment only when the deployment points yet to an
// old version of the image tag.
func (t *Tag) updateDeployments(ctx context.Context, it *imagtagv1.Tag) error {
	klog.Infof("processing deployments for tag %s/%s", it.Namespace, it.Name)
	deploys, err := t.deplis.Deployments(it.Namespace).List(labels.Everything())
	if err != nil {
		return err
	}
	for _, dep := range deploys {
		if _, ok := dep.Annotations["image-tag"]; !ok {
			continue
		}

		usesTag := false
		for _, cont := range dep.Spec.Template.Spec.Containers {
			if cont.Image != it.Name {
				continue
			}
			usesTag = true
			break
		}

		if !usesTag {
			continue
		}

		if dep.Spec.Template.Annotations == nil {
			dep.Spec.Template.Annotations = map[string]string{}
		}
		updatedAt := it.Status.LastUpdatedAt.String()

		// pod is already using the last image tag
		if dep.Spec.Template.Annotations[it.Name] == updatedAt {
			continue
		}

		klog.Infof("trigger redeployment for deploy %s/%s", dep.Namespace, dep.Name)
		dep.Spec.Template.Annotations[it.Name] = updatedAt
		if _, err := t.corcli.AppsV1().Deployments(dep.Namespace).Update(
			ctx, dep, metav1.UpdateOptions{},
		); err != nil {
			return err
		}
	}
	return nil
}

// Delete manages an image tag deletion. XXX we might need this function
// to clean up dangling objects associated with an tag but for now this
// is a no-op.
func (t *Tag) Delete(ctx context.Context, namespace, name string) error {
	return nil
}

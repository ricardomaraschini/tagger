package services

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corecli "k8s.io/client-go/kubernetes"
	aplist "k8s.io/client-go/listers/apps/v1"
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

// ValidateTagGeneration checks if tag's spec information is valid. Generation
// may be set to any already imported generation or to a new one (last imported
// generation + 1).
func (t *Tag) ValidateTagGeneration(tag imagtagv1.Tag) error {
	validGens := []int64{0}
	if len(tag.Status.References) > 0 {
		validGens = []int64{tag.Status.References[0].Generation + 1}
		for _, ref := range tag.Status.References {
			validGens = append(validGens, ref.Generation)
		}
	}
	for _, gen := range validGens {
		if gen != tag.Spec.Generation {
			continue
		}
		return nil
	}
	return fmt.Errorf("generation must be one of: %s", fmt.Sprint(validGens))
}

// CurrentReferenceForTag returns the image reference this tag is pointing to.
// If we can't find the image tag by namespace and name an empty string is
// returned instead. Image tag generation in status points to the current
// generation.
func (t *Tag) CurrentReferenceForTag(namespace, name string) (string, error) {
	it, err := t.taglis.Tags(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	for _, hashref := range it.Status.References {
		if hashref.Generation != it.Status.Generation {
			continue
		}
		return hashref.ImageReference, nil
	}
	return "", fmt.Errorf("generation does not exist")
}

// PatchForDeployment creates a patch to be applied on top of a deployment
// in order to keep track of what version of all image tags it is using.
func (t *Tag) PatchForDeployment(deploy v1.Deployment) ([]jsonpatch.JsonPatchOperation, error) {
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

		// if current generation does not exist yet it means
		// we can't add the proper annotation, just move on.
		if t.tagNotImported(is) {
			continue
		}

		// we now the generation is already imported, add an
		// annotation to the deployment pointing to it
		newAnnotations[is.Name] = fmt.Sprint(is.Status.Generation)
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
	changedData, err := json.Marshal(changed)
	if err != nil {
		return nil, err
	}

	patch, err := jsonpatch.CreatePatch(origData, changedData)
	if err != nil {
		return nil, err
	}

	// always return nil instead of an empty slice.
	if len(patch) == 0 {
		return nil, nil
	}
	return patch, nil
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

	// TODO InitContainers, EphemeralContainers.
	nconts := []corev1.Container{}
	for _, c := range pod.Spec.Containers {
		ref, err := t.CurrentReferenceForTag(pod.Namespace, c.Image)
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

	// always return nil instead of an empty slice.
	if len(patch) == 0 {
		return nil, nil
	}
	return patch, nil
}

// tagNotImported returs true if tag generation has not yet been imported.
func (t *Tag) tagNotImported(it *imagtagv1.Tag) bool {
	for _, hashref := range it.Status.References {
		if hashref.Generation == it.Status.Generation {
			return false
		}
	}
	return true
}

// prependHashReference prepends ref into refs. The resulting slice
// contains at most 5 references.
func (t *Tag) prependHashReference(
	ref imagtagv1.HashReference,
	refs []imagtagv1.HashReference,
) []imagtagv1.HashReference {
	newRefs := []imagtagv1.HashReference{ref}
	newRefs = append(newRefs, refs...)
	// XXX make this configurable.
	if len(newRefs) > 5 {
		newRefs = newRefs[:5]
	}
	return newRefs
}

// Update manages image tag updates, assuring we have the tag imported.
// Beware that we change Tag in place before updating it on api server,
// i.e. use DeepCopy() before passing the image tag in.
func (t *Tag) Update(ctx context.Context, it *imagtagv1.Tag) error {
	importNeeded := t.tagNotImported(it)
	if importNeeded {
		klog.Infof("importing tag %s/%s", it.Namespace, it.Name)
		hashref, err := t.impsvc.ImportTag(ctx, it, it.Namespace)
		if err != nil {
			return fmt.Errorf("fail to import tag: %w", err)
		}
		it.Status.References = t.prependHashReference(
			hashref, it.Status.References,
		)
	}

	deployedMismatch := it.Spec.Generation != it.Status.Generation
	if importNeeded || deployedMismatch {
		it.Status.Generation = it.Spec.Generation
		var err error
		if it, err = t.tagcli.ImagesV1().Tags(it.Namespace).Update(
			ctx, it, metav1.UpdateOptions{},
		); err != nil {
			return fmt.Errorf("error updating image stream: %w", err)
		}
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
		gen := fmt.Sprint(it.Status.Generation)

		// pod is already using the right image tag
		if dep.Spec.Template.Annotations[it.Name] == gen {
			continue
		}

		dep.Spec.Template.Annotations[it.Name] = gen
		if _, err := t.corcli.AppsV1().Deployments(dep.Namespace).Update(
			ctx, dep, metav1.UpdateOptions{},
		); err != nil {
			return err
		}
		klog.Infof("triggered redeployment for %s/%s", dep.Namespace, dep.Name)
	}
	return nil
}

// Delete manages an image tag deletion. XXX we might need this function
// to clean up dangling objects associated with an tag but for now this
// is a no-op.
func (t *Tag) Delete(ctx context.Context, namespace, name string) error {
	return nil
}

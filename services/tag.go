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

// isGenerationValid returns if spec property Generation is set to a valid
// value. It must point to the last imported Generation or to the next
// Generation (last generation + 1).
func (t *Tag) isGenerationValid(tag imagtagv1.Tag) error {
	validGens := []int64{0}
	if len(tag.Status.References) > 0 {
		lastGen := tag.Status.References[0].Generation
		validGens = []int64{lastGen, lastGen + 1}
	}
	for _, gen := range validGens {
		if gen != tag.Spec.Generations {
			continue
		}
		return nil
	}
	return fmt.Errorf("'generation' must be in %s", fmt.Sprint(validGens))
}

// isDeployedGenerationValid makes sure spec's DeployedGeneration points
// to a generation tha has been previously imported or is zeroed in case
// no generation is present.
func (t *Tag) isDeployedGenerationValid(tag imagtagv1.Tag) error {
	dplGens := []int64{0}
	if len(tag.Status.References) > 0 {
		dplGens = []int64{}
		for _, ref := range tag.Status.References {
			dplGens = append([]int64{ref.Generation}, dplGens...)
		}
	}
	for _, gen := range dplGens {
		if gen != tag.Spec.DeployedGeneration {
			continue
		}
		return nil
	}
	return fmt.Errorf("'deployedGeneration' must be in %s", fmt.Sprint(dplGens))
}

// ValidateTag checks if tag's spec information is valid.
func (t *Tag) ValidateTag(tag imagtagv1.Tag) error {
	if err := t.isGenerationValid(tag); err != nil {
		return err
	}
	return t.isDeployedGenerationValid(tag)
}

// DeployedReferenceForTag returns the docker reference we are using when
// deploying for for an image tag. If the image being deployed does not
// exist on image tag status an error is returned, if image tag does not
// exist empty value is returned.
func (t *Tag) DeployedReferenceForTag(namespace, name string) (string, error) {
	it, err := t.taglis.Tags(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	for _, hashref := range it.Status.References {
		if hashref.Generation != it.Status.DeployedGeneration {
			continue
		}
		return hashref.ImageReference, nil
	}
	return "", fmt.Errorf("deployed generation does not exist anymore")
}

// PatchForDeployment creates a patch to be applied on top of a deployment
// in order to keep track of what version of all image tags it is using.
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

		// if deployed generation does not exist yet it means
		// we can't add the proper annotation, just move on.
		if t.tagNotImported(is) {
			continue
		}

		newAnnotations[is.Name] = fmt.Sprint(is.Status.DeployedGeneration)
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
		ref, err := t.DeployedReferenceForTag(pod.Namespace, c.Image)
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

// tagNotImported returs true if tag generation has not yet been imported.
func (t *Tag) tagNotImported(it *imagtagv1.Tag) bool {
	for _, hashref := range it.Status.References {
		if hashref.Generation == it.Spec.Generations {
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

	deployedMismatch := it.Spec.DeployedGeneration != it.Status.DeployedGeneration
	if importNeeded || deployedMismatch {
		it.Status.DeployedGeneration = it.Spec.DeployedGeneration
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
		gen := fmt.Sprint(it.Status.DeployedGeneration)

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

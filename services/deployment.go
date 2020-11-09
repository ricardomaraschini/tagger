package services

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corecli "k8s.io/client-go/kubernetes"
	aplist "k8s.io/client-go/listers/apps/v1"

	taglist "github.com/ricardomaraschini/tagger/imagetags/generated/listers/imagetags/v1"
	imagtagv1 "github.com/ricardomaraschini/tagger/imagetags/v1"
)

// Deployment gather all actions related to deployment objects.
type Deployment struct {
	corcli corecli.Interface
	deplis aplist.DeploymentLister
	taglis taglist.TagLister
}

// NewDeployment returns a handler for all deployment related services.
func NewDeployment(
	corcli corecli.Interface,
	deplis aplist.DeploymentLister,
	taglis taglist.TagLister,
) *Deployment {
	return &Deployment{
		corcli: corcli,
		deplis: deplis,
		taglis: taglis,
	}
}

// UpdateDeploymentsForTag updates all deployments using provided tag. Triggers
// redeployment on deployments that have changed.
func (d *Deployment) UpdateDeploymentsForTag(ctx context.Context, it *imagtagv1.Tag) error {
	deploys, err := d.DeploymentsForTag(ctx, it)
	if err != nil {
		return err
	}
	for _, dep := range deploys {
		if err := d.Update(ctx, dep); err != nil {
			return err
		}
	}
	return nil
}

// DeploymentsForTag returns all deployments on tag's namespace that leverage
// the provided tag.
func (d *Deployment) DeploymentsForTag(
	ctx context.Context, it *imagtagv1.Tag,
) ([]*appsv1.Deployment, error) {
	deploys, err := d.deplis.Deployments(it.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var deps []*appsv1.Deployment
	for _, dep := range deploys {
		if _, ok := dep.Annotations["image-tag"]; !ok {
			continue
		}

		for _, cont := range dep.Spec.Template.Spec.Containers {
			if cont.Image != it.Name {
				continue
			}
			deps = append(deps, dep)
			break
		}
	}
	return deps, nil
}

// Update verifies if the provided deployment leverages tags, if affirmative it
// creates an annotation into its template pointing to reference pointed by the
// tag. TODO add other containers here as well.
func (d *Deployment) Update(ctx context.Context, dep *appsv1.Deployment) error {
	if _, ok := dep.Annotations["image-tag"]; !ok {
		return nil
	}

	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}

	changed := false
	for _, cont := range dep.Spec.Template.Spec.Containers {
		it, err := d.taglis.Tags(dep.Namespace).Get(cont.Image)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return err
		}

		tw := TagWrapper{it}
		ref := tw.CurrentReferenceForTag()
		if ref == "" {
			continue
		}

		// pod is already using the right image reference.
		if dep.Spec.Template.Annotations[it.Name] == ref {
			continue
		}

		dep.Spec.Template.Annotations[it.Name] = ref
		changed = true
	}

	if !changed {
		return nil
	}

	if _, err := d.corcli.AppsV1().Deployments(dep.Namespace).Update(
		ctx, dep, metav1.UpdateOptions{},
	); err != nil {
		return err
	}
	return nil
}

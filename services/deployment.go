package services

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	corecli "k8s.io/client-go/kubernetes"
	aplist "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"

	imagtagv1 "github.com/ricardomaraschini/tagger/infra/tags/v1"
	"github.com/ricardomaraschini/tagger/infra/tags/v1/gen/informers/externalversions"
	taglist "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/listers/tags/v1"
)

// Deployment gather all actions related to deployment objects.
type Deployment struct {
	corcli corecli.Interface
	corinf informers.SharedInformerFactory
	deplis aplist.DeploymentLister
	taglis taglist.TagLister
}

// NewDeployment returns a handler for all deployment related services. I have
// chosen to go with a lazy approach here, you can pass or omit (nil) the arguments,
// it is up to the caller to decide what is needed for each specific case. So far
// this is the best approach, I still plan to review this.
func NewDeployment(
	corcli corecli.Interface,
	corinf informers.SharedInformerFactory,
	taginf externalversions.SharedInformerFactory,
) *Deployment {
	var deplis aplist.DeploymentLister
	var taglis taglist.TagLister
	if corinf != nil {
		deplis = corinf.Apps().V1().Deployments().Lister()
	}
	if taginf != nil {
		taglis = taginf.Images().V1().Tags().Lister()
	}

	return &Deployment{
		corcli: corcli,
		corinf: corinf,
		deplis: deplis,
		taglis: taglis,
	}
}

// UpdateDeploymentsForTag updates all deployments using provided tag. Triggers
// redeployment on Deployments that need (some image reference has changed).
func (d *Deployment) UpdateDeploymentsForTag(ctx context.Context, it *imagtagv1.Tag) error {
	deploys, err := d.DeploymentsForTag(ctx, it)
	if err != nil {
		return fmt.Errorf("fail to get deployments for tag: %w", err)
	}
	for _, dep := range deploys {
		if err := d.Sync(ctx, dep); err != nil {
			return fmt.Errorf("fail to sync deployment: %w", err)
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
		return nil, fmt.Errorf("fail to list deployments: %w", err)
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

// Sync verifies if the provided deployment leverages tags, if affirmative it
// creates an annotation into its template pointing to reference pointed by the
// tag. TODO add other containers here as well.
func (d *Deployment) Sync(ctx context.Context, dep *appsv1.Deployment) error {
	if _, ok := dep.Annotations["image-tag"]; !ok {
		return nil
	}

	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}

	changed := false
	if _, ok := dep.Spec.Template.Annotations["image-tag"]; !ok {
		changed = true
		dep.Spec.Template.Annotations["image-tag"] = "true"
	}

	for _, cont := range dep.Spec.Template.Spec.Containers {
		it, err := d.taglis.Tags(dep.Namespace).Get(cont.Image)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("unable to get tag: %w", err)
		}

		ref := it.CurrentReferenceForTag()
		if ref == "" {
			continue
		}

		if dep.Spec.Template.Annotations[it.Name] != ref {
			dep.Spec.Template.Annotations[it.Name] = ref
			changed = true
		}
	}

	if !changed {
		return nil
	}

	_, err := d.corcli.AppsV1().Deployments(dep.Namespace).Update(
		ctx, dep, metav1.UpdateOptions{},
	)
	return err
}

// Get returns a deployment by namespace/name pair.
func (d *Deployment) Get(ctx context.Context, ns, name string) (*appsv1.Deployment, error) {
	dep, err := d.deplis.Deployments(ns).Get(name)
	if err != nil {
		return nil, fmt.Errorf("unable get deployment: %w", err)
	}
	return dep.DeepCopy(), nil
}

// AddEventHandler adds a handler to Deployment related events.
func (d *Deployment) AddEventHandler(handler cache.ResourceEventHandler) {
	d.corinf.Apps().V1().Deployments().Informer().AddEventHandler(handler)
}

package controllers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	imagtagv1beta1 "github.com/ricardomaraschini/tagger/infra/tags/v1beta1"
)

// DeploymentSyncer abstraction exists to make testing easier. You most
// likely wanna see Deployment struct under services/deployment.go for
// a concrete implementation of this.
type DeploymentSyncer interface {
	Sync(context.Context, *appsv1.Deployment) error
	UpdateDeploymentsForTag(context.Context, *imagtagv1beta1.Tag) error
	Get(context.Context, string, string) (*appsv1.Deployment, error)
	AddEventHandler(cache.ResourceEventHandler)
}

// Deployment controller handles events related to deployment. Here we
// also observe events related to tags as we need to update deployments
// whenever a tag is updated.
type Deployment struct {
	depsvc DeploymentSyncer
	tagsvc TagSyncer
	queue  workqueue.DelayingInterface
	appctx context.Context
}

// NewDeployment returns a new controller for Deployments. This controller
// keeps track of deployments being created and assure that they contain the
// right annotations if they leverage tags. Tags are also observed by this
// controller so when they get updates we also update all Deployments that
// leverage a Tag. Events for both Deployments and Tags are placed on the
// same workqueue. Once the annotations are in place it is time for the pod
// controller to take the annotations into 'image' property of the pods.
func NewDeployment(depsvc DeploymentSyncer, tagsvc TagSyncer) *Deployment {
	ctrl := &Deployment{
		queue:  workqueue.NewDelayingQueue(),
		depsvc: depsvc,
		tagsvc: tagsvc,
	}
	depsvc.AddEventHandler(ctrl.handler("deployment"))
	tagsvc.AddEventHandler(ctrl.handler("tag"))
	return ctrl
}

// Name returns a name identifier for this controller.
func (d *Deployment) Name() string {
	return "deployment"
}

// RequiresLeaderElection returns if this controller requires or not a
// leader lease to run. We run only once across multiple instances of
// tagger so we do require to be a leader.
func (d *Deployment) RequiresLeaderElection() bool {
	return true
}

// enqueueEvent enqueues an event. Receives a string indicating the event
// source kind (deployment or tag) and uses it when creating a queue key.
// Keys inside the queue are stored as "kind/namespace/name".
func (d *Deployment) enqueueEvent(kind string, o interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(o)
	if err != nil {
		klog.Errorf("fail to enqueue event: %v : %s", o, err)
		return
	}
	key = fmt.Sprintf("%s/%s", kind, key)
	d.queue.Add(key)
}

// handler returns a event handler that will be called by the informer
// whenever a Deployment or a Tag related event occurs. This handler
// enqueues everything in our work queue.
func (d *Deployment) handler(kind string) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			d.enqueueEvent(kind, o)
		},
		UpdateFunc: func(o, n interface{}) {
			d.enqueueEvent(kind, o)
		},
		DeleteFunc: func(o interface{}) {},
	}
}

// parseEventKey parses an event key and return the kind ("tag" or "deployment"),
// the namespace, and the name of the object that originated the event. rawevt
// is expected to be a string in the format "kind/namespace/name". We use empty
// interface here to make integration with workqueue cleaner.
func (d *Deployment) parseEventKey(rawevt interface{}) (string, string, string, error) {
	strevt, ok := rawevt.(string)
	if !ok {
		return "", "", "", fmt.Errorf("event is not a string: %v", rawevt)
	}

	slices := strings.SplitN(strevt, "/", 3)
	if len(slices) != 3 {
		return "", "", "", fmt.Errorf("event is invalid: %v", rawevt)
	}

	// we expect events only for kinds "deployment" and "tag".
	if slices[0] != "deployment" && slices[0] != "tag" {
		return "", "", "", fmt.Errorf("invalid event kind: %v", rawevt)
	}

	return slices[0], slices[1], slices[2], nil
}

// eventProcessor reads our events calling syncDeployment or syncTag for all of
// them. Events on the queue are expected to be a string in "kind/namespace/name"
// format.
func (d *Deployment) eventProcessor(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		rawevt, end := d.queue.Get()
		if end {
			return
		}

		kind, namespace, name, err := d.parseEventKey(rawevt)
		if err != nil {
			klog.Errorf("error parsing event: %s", err)
			d.queue.Done(rawevt)
			continue
		}

		syncfn := d.syncDeployment
		if kind == "tag" {
			syncfn = d.syncTag
		}

		klog.Infof("processing event %v", rawevt)
		if err := syncfn(namespace, name); err != nil {
			klog.Errorf("error processing %v: %v", rawevt, err)
			d.queue.Done(rawevt)
			d.queue.AddAfter(rawevt, 5*time.Second)
			continue
		}
		klog.Infof("processed event %v", rawevt)

		d.queue.Done(rawevt)
	}
}

// syncTag process an event for a tag. We look for all deployments leveraging
// the tag and update them to use the right generation.
func (d *Deployment) syncTag(namespace, name string) error {
	ctx, cancel := context.WithTimeout(d.appctx, time.Minute)
	defer cancel()

	it, err := d.tagsvc.Get(ctx, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return d.depsvc.UpdateDeploymentsForTag(ctx, it)
}

// syncDeployment process an event for a deployment. We allow ten seconds
// per Deployment update.
func (d *Deployment) syncDeployment(namespace, name string) error {
	ctx, cancel := context.WithTimeout(d.appctx, 10*time.Second)
	defer cancel()

	dep, err := d.depsvc.Get(ctx, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return d.depsvc.Sync(ctx, dep)
}

// Start starts the controller's event loop.
func (d *Deployment) Start(ctx context.Context) error {
	// appctx is the 'keep going' context, if it is cancelled
	// everything we might be doing should stop.
	d.appctx = ctx

	var wg sync.WaitGroup
	wg.Add(1)
	go d.eventProcessor(&wg)

	// wait until it is time to die.
	<-d.appctx.Done()

	d.queue.ShutDown()
	wg.Wait()
	return nil
}

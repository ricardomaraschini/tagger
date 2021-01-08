package controllers

import (
	"context"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	coreinf "k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

// DeploymentGetterSyncer abstraction exists to make testing easier. You
// most likely wanna see Deployment struct under services/deployment.go
// for a concrete implementation of this.
type DeploymentGetterSyncer interface {
	Sync(context.Context, *appsv1.Deployment) error
	Get(context.Context, string, string) (*appsv1.Deployment, error)
}

// Deployment controller handles events related to deployment creations.
type Deployment struct {
	depsvc DeploymentGetterSyncer
	queue  workqueue.DelayingInterface
	appctx context.Context
}

// NewDeployment returns a new controller for Deployments. This controller
// keeps track of deployments being created and assure that they contain the
// right annotations if they leverage tags.
func NewDeployment(
	inf coreinf.SharedInformerFactory, depsvc DeploymentGetterSyncer,
) *Deployment {
	ctrl := &Deployment{
		queue:  workqueue.NewDelayingQueue(),
		depsvc: depsvc,
	}
	inf.Apps().V1().Deployments().Informer().AddEventHandler(ctrl.handlers())
	return ctrl
}

// Name returns a name identifier for this controller.
func (d *Deployment) Name() string {
	return "deployment"
}

// enqueueEvent generates a key using "namespace/name" for the event received
// and then enqueues this index to be processed.
func (d *Deployment) enqueueEvent(o interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(o)
	if err != nil {
		klog.Errorf("fail to enqueue event: %v : %s", o, err)
		return
	}
	d.queue.Add(key)
}

// handlers return a event handler that will be called by the informer
// whenever an event occurs. This handler basically enqueues everything
// in our work queue. There is no handler for deployments deletion, we
// don't care about deletes just yet.
func (d *Deployment) handlers() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			d.enqueueEvent(o)
		},
		UpdateFunc: func(o, n interface{}) {
			d.enqueueEvent(o)
		},
		DeleteFunc: func(o interface{}) {},
	}
}

// eventProcessor reads our events calling syncDeployment for all of them.
func (d *Deployment) eventProcessor(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		evt, end := d.queue.Get()
		if end {
			return
		}

		namespace, name, err := cache.SplitMetaNamespaceKey(evt.(string))
		if err != nil {
			klog.Errorf("invalid event received %s: %s", evt, err)
			d.queue.Done(evt)
			continue
		}

		klog.Infof("received event for deployment: %s", evt)
		if err := d.syncDeployment(namespace, name); err != nil {
			klog.Errorf("error processing deployment %s: %v", evt, err)
			d.queue.Done(evt)
			d.queue.AddAfter(evt, 5*time.Second)
			continue
		}

		klog.Infof("event for deployment %s processed", evt)
		d.queue.Done(evt)
	}
}

// syncDeployment process an event for a deployment. We allow one minute
// per Deployment update.
func (d *Deployment) syncDeployment(namespace, name string) error {
	ctx, cancel := context.WithTimeout(d.appctx, time.Minute)
	defer cancel()

	dep, err := d.depsvc.Get(ctx, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	dep = dep.DeepCopy()
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

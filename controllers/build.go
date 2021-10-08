// Copyright 2020 The Tagger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	shpwv1alpha1 "github.com/shipwright-io/build/pkg/apis/build/v1alpha1"
)

// BuildRunSyncer is used to sync a BuildRun with a Tag. For the concrete
// implementation please see services/build.go file.
type BuildRunSyncer interface {
	AddEventHandler(cache.ResourceEventHandler)
	Sync(context.Context, *shpwv1alpha1.BuildRun) error
	Get(context.Context, string, string) (*shpwv1alpha1.BuildRun, error)
}

// Build controller handles events related to shipwright BuildRuns objects.
// This controller is an attempt to create or update Tag representations for
// each BuildRun in the system.
type Build struct {
	queue  workqueue.DelayingInterface
	appctx context.Context
	svc    BuildRunSyncer
}

// NewBuild returns a new controler for shipwright's BuildRun related events.
func NewBuild(svc BuildRunSyncer) *Build {
	ctrl := &Build{
		queue: workqueue.NewDelayingQueue(),
		svc:   svc,
	}
	svc.AddEventHandler(ctrl.handler())
	return ctrl
}

// Name returns a name identifier for this controller.
func (b *Build) Name() string {
	return "build"
}

// RequiresLeaderElection returns if this controller requires or not a leader
// lease to run. We run only once across multiple instances of tagger so we do
// require to be the leader.
func (b *Build) RequiresLeaderElection() bool {
	return true
}

// enqueueEvent enqueues an event into our work queue. This function enqueues
// a string in the format <namespace>/<name> for the BuildRun.
func (b *Build) enqueueEvent(o interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(o)
	if err != nil {
		klog.Errorf("fail to enqueue event: %v : %s", o, err)
		return
	}
	b.queue.Add(key)
}

// handler returns a event handler that will be called by the informer whenever
// a BuildRun related event occurs. The returned handles calls enqueueEvent().
func (b *Build) handler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			b.enqueueEvent(o)
		},
		UpdateFunc: func(o, n interface{}) {
			b.enqueueEvent(o)
		},
		DeleteFunc: func(o interface{}) {
			b.enqueueEvent(o)
		},
	}
}

// eventProcessor reads our events calling syncBuildRun for all of them. Events on
// the queue are expected to be a string in <namespace>/<name> format.
func (b *Build) eventProcessor(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		evt, end := b.queue.Get()
		if end {
			return
		}

		namespace, name, err := cache.SplitMetaNamespaceKey(evt.(string))
		if err != nil {
			klog.Errorf("invalid event received %s: %s", evt, err)
			b.queue.Done(evt)
			return
		}

		klog.Infof("received event for buildrun: %s", evt)
		if err := b.syncBuildRun(namespace, name); err != nil {
			klog.Errorf("error processing buildrun %s: %v", evt, err)
			b.queue.Done(evt)
			b.queue.AddAfter(evt, 10*time.Second)
			return
		}

		klog.Infof("event for buildrun %s processed", evt)
		b.queue.Done(evt)
	}
}

// syncBuildRun process an event for a BuildRun. Fetches the object and pass it down
// to the service layer. XXX If BuildRun has been deleted we do not delete the tag,
// we simply ignore the event. Five seconds is the timeout for each event.
func (b *Build) syncBuildRun(namespace, name string) error {
	ctx, cancel := context.WithTimeout(b.appctx, 5*time.Second)
	defer cancel()

	br, err := b.svc.Get(ctx, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return b.svc.Sync(ctx, br)
}

// Start starts the controller's event loop.
func (b *Build) Start(ctx context.Context) error {
	// appctx is the 'keep going' context, if it is cancelled everything we might
	// be doing should stop. appctx is probably has its life ended as soon as the
	// binary receives a signal.
	b.appctx = ctx

	var wg sync.WaitGroup
	wg.Add(1)
	go b.eventProcessor(&wg)

	// wait until it is time to die.
	<-b.appctx.Done()

	b.queue.ShutDown()
	wg.Wait()
	return nil
}

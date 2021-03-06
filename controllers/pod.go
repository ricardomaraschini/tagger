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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

// PodSyncer abstraction exists to make testing easier. You most
// likely wanna see Pod struct under services/pod.go for a concrete
// implementation of this.
type PodSyncer interface {
	Sync(context.Context, *corev1.Pod) error
	Get(context.Context, string, string) (*corev1.Pod, error)
	AddEventHandler(cache.ResourceEventHandler)
}

// Pod controller handles events related to pods.
type Pod struct {
	podsvc PodSyncer
	queue  workqueue.RateLimitingInterface
	appctx context.Context
}

// NewPod returns a new controller for Pods.
func NewPod(podsvc PodSyncer) *Pod {
	ratelimit := workqueue.NewItemExponentialFailureRateLimiter(time.Second, time.Minute)
	ctrl := &Pod{
		queue:  workqueue.NewRateLimitingQueue(ratelimit),
		podsvc: podsvc,
	}
	podsvc.AddEventHandler(ctrl.handler())
	return ctrl
}

// Name returns a name identifier for this controller.
func (p *Pod) Name() string {
	return "pod"
}

// RequiresLeaderElection returns if this controller requires or not a
// leader lease to run.
func (p *Pod) RequiresLeaderElection() bool {
	return true
}

// enqueueEvent enqueues an event into our workqueue.
func (p *Pod) enqueueEvent(o interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(o)
	if err != nil {
		klog.Errorf("fail to enqueue event: %v : %s", o, err)
		return
	}
	p.queue.AddRateLimited(key)
}

// handler returns a event handler that will be called by the informer
// whenever a Pod event occurs.
func (p *Pod) handler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			p.enqueueEvent(o)
		},
		UpdateFunc: func(o, n interface{}) {
			p.enqueueEvent(o)
		},
		DeleteFunc: func(o interface{}) {},
	}
}

// eventProcessor reads our events calling syncPod for all of them. Events
// on the queue are expected to be strings in "namespace/name" format.
func (p *Pod) eventProcessor(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		rawevt, end := p.queue.Get()
		if end {
			return
		}

		namespace, name, err := cache.SplitMetaNamespaceKey(rawevt.(string))
		if err != nil {
			klog.Errorf("error parsing event: %s", err)
			p.queue.Done(rawevt)
			continue
		}

		klog.Infof("processing event for pod %v", rawevt)
		if err := p.syncPod(namespace, name); err != nil {
			klog.Errorf("error processing event for pod %v: %v", rawevt, err)
			p.queue.Done(rawevt)
			p.queue.AddAfter(rawevt, 5*time.Second)
			continue
		}
		klog.Infof("processed event for pod %v", rawevt)

		p.queue.Done(rawevt)
	}
}

// syncPod process an event for a pod. We retrieve the pod object using
// PodSyncer and call PodSyncer.Sync. If the pod does not exist we ignore
// the event.
func (p *Pod) syncPod(namespace, name string) error {
	ctx, cancel := context.WithTimeout(p.appctx, 5*time.Second)
	defer cancel()

	pod, err := p.podsvc.Get(ctx, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	pod = pod.DeepCopy()
	return p.podsvc.Sync(ctx, pod)
}

// Start starts the controller's event loop.
func (p *Pod) Start(ctx context.Context) error {
	// appctx is the 'keep going' context, if it is cancelled
	// everything we might be doing should stop.
	p.appctx = ctx

	var wg sync.WaitGroup
	wg.Add(1)
	go p.eventProcessor(&wg)

	// wait until it is time to die.
	<-p.appctx.Done()

	p.queue.ShutDown()
	wg.Wait()
	return nil
}

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

	imgv1b1 "github.com/ricardomaraschini/tagger/infra/images/v1beta1"
	"github.com/ricardomaraschini/tagger/infra/metrics"
)

// ImageImportSyncer abstraction exists to make testing easier. You most likely wanna
// see Image struct under services/imageimport.go for a concrete implementation of this.
type ImageImportSyncer interface {
	Sync(context.Context, *imgv1b1.ImageImport) error
	Get(context.Context, string, string) (*imgv1b1.ImageImport, error)
	AddEventHandler(cache.ResourceEventHandler)
}

// ImageImport controller handles events related to ImageImports. It starts and receives
// events from the informer, calling appropriate functions on our concrete services
// layer implementation.
type ImageImport struct {
	queue  workqueue.RateLimitingInterface
	tisvc  ImageImportSyncer
	appctx context.Context
	tokens chan bool
}

// NewImageImport returns a new controller for ImageImports. This controller runs image
// imports in parallel, at a given time we can have at max "workers" distinct imports being
// processed.
func NewImageImport(tisvc ImageImportSyncer) *ImageImport {
	ratelimit := workqueue.NewItemExponentialFailureRateLimiter(time.Second, time.Minute)
	ctrl := &ImageImport{
		queue:  workqueue.NewRateLimitingQueue(ratelimit),
		tisvc:  tisvc,
		tokens: make(chan bool, 10),
	}
	tisvc.AddEventHandler(ctrl.handlers())
	return ctrl
}

// Name returns a name identifier for this controller.
func (t *ImageImport) Name() string {
	return "image import"
}

// RequiresLeaderElection returns if this controller requires or not a
// leader lease to run.
func (t *ImageImport) RequiresLeaderElection() bool {
	return true
}

// enqueueEvent generates a key using "namespace/name" for the event received
// and then enqueues this index to be processed.
func (t *ImageImport) enqueueEvent(o interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(o)
	if err != nil {
		klog.Errorf("fail to enqueue event: %v : %s", o, err)
		return
	}
	t.queue.AddRateLimited(key)
}

// handlers return a event handler that will be called by the informer
// whenever an event occurs. This handler basically enqueues everything
// in our work queue.
func (t *ImageImport) handlers() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			t.enqueueEvent(o)
		},
		UpdateFunc: func(o, n interface{}) {
			t.enqueueEvent(o)
		},
		DeleteFunc: func(o interface{}) {
			t.enqueueEvent(o)
		},
	}
}

// eventProcessor reads our events calling syncImageImport for all of them. Uses
// t.tokens to control how many imports are processed in parallel.
func (t *ImageImport) eventProcessor(wg *sync.WaitGroup) {
	var running sync.WaitGroup
	defer wg.Done()
	for {
		evt, end := t.queue.Get()
		if end {
			klog.Info("queue closed, awaiting for running workers")
			running.Wait()
			klog.Info("all running workers finished")
			return
		}

		t.tokens <- true
		running.Add(1)
		go func() {
			metrics.ActiveWorkers.Inc()
			defer func() {
				<-t.tokens
				running.Done()
				metrics.ActiveWorkers.Dec()
			}()

			namespace, name, err := cache.SplitMetaNamespaceKey(evt.(string))
			if err != nil {
				klog.Errorf("invalid event received %s: %s", evt, err)
				t.queue.Done(evt)
				return
			}

			klog.Infof("received event for image import: %s", evt)
			if err := t.syncImageImport(namespace, name); err != nil {
				klog.Errorf("error processing image import %s: %v", evt, err)
				t.queue.Done(evt)
				t.queue.AddRateLimited(evt)
				return
			}

			klog.Infof("event for image import %s processed", evt)
			t.queue.Done(evt)
			t.queue.Forget(evt)
		}()
	}
}

// syncImageImport process an event for an image import. A max of three minutes is
// allowed per image import.
func (t *ImageImport) syncImageImport(namespace, name string) error {
	ctx, cancel := context.WithTimeout(t.appctx, 3*time.Minute)
	defer cancel()

	it, err := t.tisvc.Get(ctx, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return t.tisvc.Sync(ctx, it)
}

// Start starts the controller's event loop.
func (t *ImageImport) Start(ctx context.Context) error {
	// appctx is the 'keep going' context, if it is cancelled
	// everything we might be doing should stop.
	t.appctx = ctx

	var wg sync.WaitGroup
	wg.Add(1)
	go t.eventProcessor(&wg)

	// wait until it is time to die.
	<-t.appctx.Done()

	t.queue.ShutDown()
	wg.Wait()
	return nil
}

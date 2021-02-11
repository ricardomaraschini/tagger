package controllers

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	imagtagv1 "github.com/ricardomaraschini/tagger/infra/tags/v1"
)

// TagSyncer abstraction exists to make testing easier. You most likely wanna
// see Tag struct under services/tag.go for a concrete implementation of this.
type TagSyncer interface {
	Sync(context.Context, *imagtagv1.Tag) error
	Get(context.Context, string, string) (*imagtagv1.Tag, error)
	AddEventHandler(cache.ResourceEventHandler)
}

// MetricReporter abstraction exists to make tests easier. You might be looking
// for its concrete implementation on services/metrics.go.
type MetricReporter interface {
	ReportWorker(bool)
}

// Tag controller handles events related to Tags. It starts and receives events
// from the informer, calling appropriate functions on our concrete services
// layer implementation.
type Tag struct {
	queue  workqueue.RateLimitingInterface
	tagsvc TagSyncer
	mtrsvc MetricReporter
	appctx context.Context
	tokens chan bool
}

// NewTag returns a new controller for Image Tags. This controller runs image
// tag imports in parallel, at a given time we can have at max "workers"
// distinct image tags being processed.
func NewTag(
	tagsvc TagSyncer,
	mtrsvc MetricReporter,
) *Tag {
	ratelimit := workqueue.NewItemExponentialFailureRateLimiter(time.Second, time.Minute)
	ctrl := &Tag{
		queue:  workqueue.NewRateLimitingQueue(ratelimit),
		tagsvc: tagsvc,
		mtrsvc: mtrsvc,
		tokens: make(chan bool, 10),
	}
	tagsvc.AddEventHandler(ctrl.handlers())
	return ctrl
}

// Name returns a name identifier for this controller.
func (t *Tag) Name() string {
	return "tag"
}

// enqueueEvent generates a key using "namespace/name" for the event received
// and then enqueues this index to be processed.
func (t *Tag) enqueueEvent(o interface{}) {
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
func (t *Tag) handlers() cache.ResourceEventHandler {
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

// eventProcessor reads our events calling syncTag for all of them.
func (t *Tag) eventProcessor(wg *sync.WaitGroup) {
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
			defer func() {
				<-t.tokens
				running.Done()
				t.mtrsvc.ReportWorker(false)
			}()
			t.mtrsvc.ReportWorker(true)

			namespace, name, err := cache.SplitMetaNamespaceKey(evt.(string))
			if err != nil {
				klog.Errorf("invalid event received %s: %s", evt, err)
				t.queue.Done(evt)
				return
			}

			klog.Infof("received event for tag: %s", evt)
			if err := t.syncTag(namespace, name); err != nil {
				klog.Errorf("error processing tag %s: %v", evt, err)
				t.queue.Done(evt)
				t.queue.AddRateLimited(evt)
				return
			}

			klog.Infof("event for tag %s processed", evt)
			t.queue.Done(evt)
			t.queue.Forget(evt)
		}()
	}
}

// syncTag process an event for an image stream. A max of three minutes is
// allowed per image stream sync.
func (t *Tag) syncTag(namespace, name string) error {
	ctx, cancel := context.WithTimeout(t.appctx, 3*time.Minute)
	defer cancel()

	it, err := t.tagsvc.Get(ctx, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	it = it.DeepCopy()
	return t.tagsvc.Sync(ctx, it)
}

// Start starts the controller's event loop.
func (t *Tag) Start(ctx context.Context) error {
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

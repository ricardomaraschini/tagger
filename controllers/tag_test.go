package controllers

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	tagfake "github.com/ricardomaraschini/tagger/imagetags/generated/clientset/versioned/fake"
	taginformer "github.com/ricardomaraschini/tagger/imagetags/generated/informers/externalversions"
	imagtagv1 "github.com/ricardomaraschini/tagger/imagetags/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

type mtrsvc struct{}

func (m mtrsvc) ReportWorker(bool) {}

type tagsvc struct {
	sync.Mutex
	db     map[string]*imagtagv1.Tag
	calls  int
	delay  time.Duration
	tagcli *tagfake.Clientset
}

func (t *tagsvc) Update(ctx context.Context, tag *imagtagv1.Tag) error {
	t.Lock()

	if t.db == nil {
		t.db = make(map[string]*imagtagv1.Tag)
	}
	idx := fmt.Sprintf("%s/%s", tag.Namespace, tag.Name)
	t.db[idx] = tag.DeepCopy()
	t.calls++

	t.Unlock()
	time.Sleep(t.delay)
	return nil
}

func (t *tagsvc) Get(ctx context.Context, ns, name string) (*imagtagv1.Tag, error) {
	return t.tagcli.ImagesV1().Tags(ns).Get(ctx, name, metav1.GetOptions{})
}

func (t *tagsvc) get(idx string) *imagtagv1.Tag {
	t.Lock()
	defer t.Unlock()
	return t.db[idx]
}

func (t *tagsvc) len() int {
	t.Lock()
	defer t.Unlock()
	return len(t.db)
}

func TestTagCreated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	tagcli := tagfake.NewSimpleClientset()
	taginf := taginformer.NewSharedInformerFactory(tagcli, time.Minute)
	svc := &tagsvc{
		tagcli: tagcli,
	}

	ctrl := NewTag(taginf, svc, mtrsvc{})
	ctrl.tokens = make(chan bool, 1)
	taginf.Start(ctx.Done())

	if !cache.WaitForCacheSync(
		ctx.Done(),
		taginf.Images().V1().Tags().Informer().HasSynced,
	) {
		cancel()
		t.Fatal("timeout waiting for caches to sync")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := ctrl.Start(ctx); err != nil {
			t.Errorf("unexpected error after start: %s", err)
		}
	}()

	tag := &imagtagv1.Tag{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "atag",
		},
		Spec: imagtagv1.TagSpec{
			From:  "centos:7",
			Cache: true,
		},
	}

	if _, err := tagcli.ImagesV1().Tags("namespace").Create(
		ctx, tag, metav1.CreateOptions{},
	); err != nil {
		t.Fatalf("error creating tag: %s", err)
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(3 * time.Second)

	if !reflect.DeepEqual(tag, svc.get("namespace/atag")) {
		t.Errorf("expected %+v, found %+v", tag, svc.db["namespace/atag"])
	}

	if svc.calls != 1 {
		t.Errorf("expected 1 call, %d calls made", svc.calls)
	}

	cancel()
	wg.Wait()
}

func TestTagUpdated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	tagcli := tagfake.NewSimpleClientset()
	taginf := taginformer.NewSharedInformerFactory(tagcli, time.Minute)
	svc := &tagsvc{
		tagcli: tagcli,
	}

	ctrl := NewTag(taginf, svc, mtrsvc{})
	ctrl.tokens = make(chan bool, 1)
	taginf.Start(ctx.Done())

	if !cache.WaitForCacheSync(
		ctx.Done(),
		taginf.Images().V1().Tags().Informer().HasSynced,
	) {
		cancel()
		t.Fatal("timeout waiting for caches to sync")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := ctrl.Start(ctx); err != nil {
			t.Errorf("unexpected error after start: %s", err)
		}
	}()

	tag := &imagtagv1.Tag{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "atag",
		},
		Spec: imagtagv1.TagSpec{
			From:  "centos:7",
			Cache: true,
		},
	}

	if _, err := tagcli.ImagesV1().Tags("namespace").Create(
		ctx, tag, metav1.CreateOptions{},
	); err != nil {
		t.Fatalf("error creating tag: %s", err)
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(3 * time.Second)

	if !reflect.DeepEqual(tag, svc.get("namespace/atag")) {
		t.Errorf("expected %+v, found %+v", tag, svc.db["namespace/atag"])
	}

	tag.Spec.From = "rhel:latest"
	if _, err := tagcli.ImagesV1().Tags("namespace").Update(
		ctx, tag, metav1.UpdateOptions{},
	); err != nil {
		t.Fatalf("error updating tag: %s", err)
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(3 * time.Second)

	if !reflect.DeepEqual(tag, svc.get("namespace/atag")) {
		t.Errorf("expected %+v, found %+v", tag, svc.db["namespace/atag"])
	}

	if svc.calls != 2 {
		t.Errorf("expected 2 calls, %d calls made", svc.calls)
	}

	cancel()
	wg.Wait()
}

func TestTagParallel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	tagcli := tagfake.NewSimpleClientset()
	taginf := taginformer.NewSharedInformerFactory(tagcli, time.Minute)
	svc := &tagsvc{
		delay:  3 * time.Second,
		tagcli: tagcli,
	}

	ctrl := NewTag(taginf, svc, mtrsvc{})
	ctrl.tokens = make(chan bool, 5)
	taginf.Start(ctx.Done())

	if !cache.WaitForCacheSync(
		ctx.Done(),
		taginf.Images().V1().Tags().Informer().HasSynced,
	) {
		cancel()
		t.Fatal("timeout waiting for caches to sync")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := ctrl.Start(ctx); err != nil {
			t.Errorf("unexpected error after start: %s", err)
		}
	}()

	for i := 0; i < 10; i++ {
		tag := &imagtagv1.Tag{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "namespace",
				Name:      fmt.Sprintf("tag-%d", i),
			},
			Spec: imagtagv1.TagSpec{
				From:  "centos:7",
				Cache: true,
			},
		}
		if _, err := tagcli.ImagesV1().Tags("namespace").Create(
			ctx, tag, metav1.CreateOptions{},
		); err != nil {
			t.Fatalf("error creating tag: %s", err)
		}
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(3 * time.Second)

	if svc.len() != 5 {
		t.Errorf("5 parallel processes expected: %d", len(svc.db))
	}

	cancel()
	wg.Wait()
}

func TestTagDeleted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	tagcli := tagfake.NewSimpleClientset()
	taginf := taginformer.NewSharedInformerFactory(tagcli, time.Minute)
	svc := &tagsvc{
		tagcli: tagcli,
	}

	ctrl := NewTag(taginf, svc, mtrsvc{})
	ctrl.tokens = make(chan bool, 1)
	taginf.Start(ctx.Done())

	if !cache.WaitForCacheSync(
		ctx.Done(),
		taginf.Images().V1().Tags().Informer().HasSynced,
	) {
		cancel()
		t.Fatal("timeout waiting for caches to sync")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := ctrl.Start(ctx); err != nil {
			t.Errorf("unexpected error after start: %s", err)
		}
	}()

	tag := &imagtagv1.Tag{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "atag",
		},
		Spec: imagtagv1.TagSpec{
			From:  "centos:7",
			Cache: true,
		},
	}

	if _, err := tagcli.ImagesV1().Tags("namespace").Create(
		ctx, tag, metav1.CreateOptions{},
	); err != nil {
		t.Fatalf("error creating tag: %s", err)
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(3 * time.Second)

	if !reflect.DeepEqual(tag, svc.get("namespace/atag")) {
		t.Errorf("expected %+v, found %+v", tag, svc.db["namespace/atag"])
	}

	if err := tagcli.ImagesV1().Tags("namespace").Delete(
		ctx, "atag", metav1.DeleteOptions{},
	); err != nil {
		t.Fatalf("error updating tag: %s", err)
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(3 * time.Second)

	if svc.calls != 1 {
		t.Errorf("expected 1 call, %d calls made", svc.calls)
	}

	cancel()
	wg.Wait()
}

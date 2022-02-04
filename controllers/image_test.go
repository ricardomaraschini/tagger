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
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	imgv1b1 "github.com/ricardomaraschini/tagger/infra/images/v1beta1"
	imgfake "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/clientset/versioned/fake"
	imginformer "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/informers/externalversions"
)

type imgsvc struct {
	sync.Mutex
	db     map[string]*imgv1b1.Image
	calls  int
	delay  time.Duration
	imgcli *imgfake.Clientset
	imginf imginformer.SharedInformerFactory
}

func (t *imgsvc) Sync(ctx context.Context, img *imgv1b1.Image) error {
	t.Lock()

	if t.db == nil {
		t.db = make(map[string]*imgv1b1.Image)
	}
	idx := fmt.Sprintf("%s/%s", img.Namespace, img.Name)
	t.db[idx] = img.DeepCopy()
	t.calls++

	t.Unlock()
	time.Sleep(t.delay)
	return nil
}

func (t *imgsvc) Get(ctx context.Context, ns, name string) (*imgv1b1.Image, error) {
	return t.imgcli.TaggerV1beta1().Images(ns).Get(ctx, name, metav1.GetOptions{})
}

func (t *imgsvc) get(idx string) *imgv1b1.Image {
	t.Lock()
	defer t.Unlock()
	return t.db[idx]
}

func (t *imgsvc) len() int {
	t.Lock()
	defer t.Unlock()
	return len(t.db)
}

func (t *imgsvc) AddEventHandler(handler cache.ResourceEventHandler) {
	t.imginf.Tagger().V1beta1().Images().Informer().AddEventHandler(handler)
}

func TestImageCreated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	imgcli := imgfake.NewSimpleClientset()
	imginf := imginformer.NewSharedInformerFactory(imgcli, time.Minute)
	svc := &imgsvc{
		imginf: imginf,
		imgcli: imgcli,
	}

	ctrl := NewImage(svc)
	ctrl.tokens = make(chan bool, 1)
	imginf.Start(ctx.Done())

	if !cache.WaitForCacheSync(
		ctx.Done(),
		imginf.Tagger().V1beta1().Images().Informer().HasSynced,
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

	img := &imgv1b1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "aimg",
		},
		Spec: imgv1b1.ImageSpec{
			From:   "centos:7",
			Mirror: true,
		},
	}

	if _, err := imgcli.TaggerV1beta1().Images("namespace").Create(
		ctx, img, metav1.CreateOptions{},
	); err != nil {
		t.Fatalf("error creating img: %s", err)
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(3 * time.Second)

	if !reflect.DeepEqual(img, svc.get("namespace/aimg")) {
		t.Errorf("expected %+v, found %+v", img, svc.db["namespace/aimg"])
	}

	if svc.calls != 1 {
		t.Errorf("expected 1 call, %d calls made", svc.calls)
	}

	cancel()
	wg.Wait()
}

func TestImageUpdated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	imgcli := imgfake.NewSimpleClientset()
	imginf := imginformer.NewSharedInformerFactory(imgcli, time.Minute)
	svc := &imgsvc{
		imginf: imginf,
		imgcli: imgcli,
	}

	ctrl := NewImage(svc)
	ctrl.tokens = make(chan bool, 1)
	imginf.Start(ctx.Done())

	if !cache.WaitForCacheSync(
		ctx.Done(),
		imginf.Tagger().V1beta1().Images().Informer().HasSynced,
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

	img := &imgv1b1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "aimg",
		},
		Spec: imgv1b1.ImageSpec{
			From:   "centos:7",
			Mirror: true,
		},
	}

	if _, err := imgcli.TaggerV1beta1().Images("namespace").Create(
		ctx, img, metav1.CreateOptions{},
	); err != nil {
		t.Fatalf("error creating img: %s", err)
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(3 * time.Second)

	if !reflect.DeepEqual(img, svc.get("namespace/aimg")) {
		t.Errorf("expected %+v, found %+v", img, svc.db["namespace/aimg"])
	}

	img.Spec.From = "rhel:latest"
	if _, err := imgcli.TaggerV1beta1().Images("namespace").Update(
		ctx, img, metav1.UpdateOptions{},
	); err != nil {
		t.Fatalf("error updating img: %s", err)
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(3 * time.Second)

	if !reflect.DeepEqual(img, svc.get("namespace/aimg")) {
		t.Errorf("expected %+v, found %+v", img, svc.db["namespace/aimg"])
	}

	if svc.calls != 2 {
		t.Errorf("expected 2 calls, %d calls made", svc.calls)
	}

	cancel()
	wg.Wait()
}

func TestImageParallel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	imgcli := imgfake.NewSimpleClientset()
	imginf := imginformer.NewSharedInformerFactory(imgcli, time.Minute)
	svc := &imgsvc{
		delay:  3 * time.Second,
		imginf: imginf,
		imgcli: imgcli,
	}

	ctrl := NewImage(svc)
	ctrl.tokens = make(chan bool, 5)
	imginf.Start(ctx.Done())

	if !cache.WaitForCacheSync(
		ctx.Done(),
		imginf.Tagger().V1beta1().Images().Informer().HasSynced,
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
		img := &imgv1b1.Image{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "namespace",
				Name:      fmt.Sprintf("img-%d", i),
			},
			Spec: imgv1b1.ImageSpec{
				From:   "centos:7",
				Mirror: true,
			},
		}
		if _, err := imgcli.TaggerV1beta1().Images("namespace").Create(
			ctx, img, metav1.CreateOptions{},
		); err != nil {
			t.Fatalf("error creating img: %s", err)
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

func TestImageDeleted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	imgcli := imgfake.NewSimpleClientset()
	imginf := imginformer.NewSharedInformerFactory(imgcli, time.Minute)
	svc := &imgsvc{
		imginf: imginf,
		imgcli: imgcli,
	}

	ctrl := NewImage(svc)
	ctrl.tokens = make(chan bool, 1)
	imginf.Start(ctx.Done())

	if !cache.WaitForCacheSync(
		ctx.Done(),
		imginf.Tagger().V1beta1().Images().Informer().HasSynced,
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

	img := &imgv1b1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "aimg",
		},
		Spec: imgv1b1.ImageSpec{
			From:   "centos:7",
			Mirror: true,
		},
	}

	if _, err := imgcli.TaggerV1beta1().Images("namespace").Create(
		ctx, img, metav1.CreateOptions{},
	); err != nil {
		t.Fatalf("error creating img: %s", err)
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(3 * time.Second)

	if !reflect.DeepEqual(img, svc.get("namespace/aimg")) {
		t.Errorf("expected %+v, found %+v", img, svc.db["namespace/aimg"])
	}

	if err := imgcli.TaggerV1beta1().Images("namespace").Delete(
		ctx, "aimg", metav1.DeleteOptions{},
	); err != nil {
		t.Fatalf("error updating img: %s", err)
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(3 * time.Second)

	if svc.calls != 1 {
		t.Errorf("expected 1 call, %d calls made", svc.calls)
	}

	cancel()
	wg.Wait()
}

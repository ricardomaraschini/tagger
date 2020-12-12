package controllers

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinf "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/pointer"
)

type depsvc struct {
	sync.Mutex
	db    map[string]*appsv1.Deployment
	calls int
}

func (d *depsvc) Update(ctx context.Context, dep *appsv1.Deployment) error {
	d.Lock()
	defer d.Unlock()

	if d.db == nil {
		d.db = make(map[string]*appsv1.Deployment)
	}
	idx := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
	d.db[idx] = dep.DeepCopy()
	d.calls++
	return nil
}

func (d *depsvc) get(idx string) *appsv1.Deployment {
	d.Lock()
	defer d.Unlock()
	return d.db[idx]
}

func TestDeploymentCreated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	corcli := fake.NewSimpleClientset()
	corinf := coreinf.NewSharedInformerFactory(corcli, time.Minute)
	svc := &depsvc{}

	ctrl := NewDeployment(corinf, svc)
	corinf.Start(ctx.Done())

	if !cache.WaitForCacheSync(
		ctx.Done(),
		corinf.Apps().V1().Deployments().Informer().HasSynced,
	) {
		cancel()
		t.Fatal("timeout waiting for caches to sync")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := ctrl.Start(ctx); err != nil {
			t.Errorf("unexpected error starting controller: %s", err)
		}
	}()

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "adeployment",
		},
	}
	if _, err := corcli.AppsV1().Deployments("namespace").Create(
		ctx, dep, metav1.CreateOptions{},
	); err != nil {
		t.Errorf("error creating deployment: %s", err)
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(time.Second)

	if !reflect.DeepEqual(dep, svc.get("namespace/adeployment")) {
		t.Errorf("expected %+v, found %+v", dep, svc.db["namespace/adeployment"])
	}

	if svc.calls != 1 {
		t.Errorf("expected 1 call, %d calls made", svc.calls)
	}

	cancel()
	wg.Wait()
}

func TestDeploymentUpdated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	corcli := fake.NewSimpleClientset()
	corinf := coreinf.NewSharedInformerFactory(corcli, time.Minute)
	svc := &depsvc{}

	ctrl := NewDeployment(corinf, svc)
	corinf.Start(ctx.Done())

	if !cache.WaitForCacheSync(
		ctx.Done(),
		corinf.Apps().V1().Deployments().Informer().HasSynced,
	) {
		cancel()
		t.Fatal("timeout waiting for caches to sync")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := ctrl.Start(ctx); err != nil {
			t.Errorf("unexpected error starting controller: %s", err)
		}
	}()

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "adeployment",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(1),
		},
	}
	if _, err := corcli.AppsV1().Deployments("namespace").Create(
		ctx, dep, metav1.CreateOptions{},
	); err != nil {
		t.Errorf("error creating deployment: %s", err)
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(time.Second)

	if !reflect.DeepEqual(dep, svc.get("namespace/adeployment")) {
		t.Errorf("expected %+v, found %+v", dep, svc.db["namespace/adeployment"])
	}

	dep.Spec.Replicas = pointer.Int32Ptr(2)
	if _, err := corcli.AppsV1().Deployments("namespace").Update(
		ctx, dep, metav1.UpdateOptions{},
	); err != nil {
		t.Errorf("error updating deployment: %s", err)
	}

	// give some room for the event to be dispatched towards the controller.
	time.Sleep(time.Second)

	if !reflect.DeepEqual(dep, svc.get("namespace/adeployment")) {
		t.Errorf("expected %+v, found %+v", dep, svc.db["namespace/adeployment"])
	}

	if svc.calls != 2 {
		t.Errorf("expected 2 calls, %d calls made", svc.calls)
	}

	cancel()
	wg.Wait()
}

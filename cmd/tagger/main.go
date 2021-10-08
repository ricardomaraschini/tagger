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

package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	coreinf "k8s.io/client-go/informers"
	corecli "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	shpwv1alpha1 "github.com/shipwright-io/build/pkg/apis/build/v1alpha1"
	shpwcli "github.com/shipwright-io/build/pkg/client/clientset/versioned"
	shpwinf "github.com/shipwright-io/build/pkg/client/informers/externalversions"

	"github.com/ricardomaraschini/tagger/controllers"
	"github.com/ricardomaraschini/tagger/infra/starter"
	itagcli "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/clientset/versioned"
	itaginf "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/informers/externalversions"
	"github.com/ricardomaraschini/tagger/services"
)

// Version holds the current binary version. Set at compile time.
var Version = "v0.0.0"

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	ctx, stop := signal.NotifyContext(
		context.Background(), syscall.SIGTERM, syscall.SIGINT,
	)
	go func() {
		<-ctx.Done()
		stop()
	}()

	klog.Info(` _|_  __,   __,  __,  _   ,_     `)
	klog.Info(`  |  /  |  /  | /  | |/  /  |    `)
	klog.Info(`  |_/\_/|_/\_/|/\_/|/|__/   |_/. `)
	klog.Info(`             /|   /|             `)
	klog.Info(`             \|   \|             `)
	klog.Info(`starting image tag controller... `)
	klog.Info(`version `, Version)

	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		klog.Fatalf("unable to read kubeconfig: %v", err)
	}

	// creates tag client and informer.
	tagcli, err := itagcli.NewForConfig(config)
	if err != nil {
		klog.Fatalf("unable to create image tag client: %v", err)
	}
	taginf := itaginf.NewSharedInformerFactory(tagcli, time.Minute)

	// creates core client and informer.
	corcli, err := corecli.NewForConfig(config)
	if err != nil {
		klog.Fatalf("unable to create core client: %v", err)
	}
	corinf := coreinf.NewSharedInformerFactory(corcli, time.Minute)

	// creates build client and informer
	shpcli, err := shpwcli.NewForConfig(config)
	if err != nil {
		klog.Fatalf("unable to create build client: %v", err)
	}
	shpinf := shpwinf.NewSharedInformerFactory(shpcli, time.Minute)

	// create our service layer
	shpsvc := services.NewBuild(corinf, tagcli, taginf, shpinf)
	tagsvc := services.NewTag(corinf, tagcli, taginf)
	tiosvc := services.NewTagIO(corinf, tagcli, taginf)
	usrsvc := services.NewUser(corcli)

	// create controller layer
	itctrl := controllers.NewTag(tagsvc)
	shctrl := controllers.NewBuild(shpsvc)
	mtctrl := controllers.NewMutatingWebHook()
	qyctrl := controllers.NewQuayWebHook(tagsvc)
	dkctrl := controllers.NewDockerWebHook(tagsvc)
	tioctr := controllers.NewTagIO(tiosvc, usrsvc)
	moctrl := controllers.NewMetric()

	// starts up all informers and waits for their cache to sync up,
	// only then we start the controllers i.e. start to process events
	// from the queue.
	corinf.Start(ctx.Done())
	taginf.Start(ctx.Done())

	syncs := []cache.InformerSynced{
		corinf.Core().V1().ConfigMaps().Informer().HasSynced,
		corinf.Core().V1().Secrets().Informer().HasSynced,
		taginf.Tagger().V1beta1().Tags().Informer().HasSynced,
	}
	if hasShipwright(shpcli) {
		// we only need to start this informer if shipwright is installed in the
		// cluster.
		shpinf.Start(ctx.Done())
		syncs = append(
			syncs,
			shpinf.Shipwright().V1alpha1().BuildRuns().Informer().HasSynced,
		)
	}

	klog.Info("waiting for caches to sync ...")
	if !cache.WaitForCacheSync(ctx.Done(), syncs...) {
		klog.Fatal("caches not syncing")
	}
	klog.Info("caches in sync, moving on.")

	st := starter.New(corcli, mtctrl, qyctrl, dkctrl, itctrl, moctrl, tioctr, shctrl)
	if err := st.Start(ctx, "tagger-leader-election"); err != nil {
		klog.Errorf("unable to start controllers: %s", err)
	}
}

// hasShipwright returns true if we can find "shipwright.io" object group installed in the
// cluster. This function inspects API groups, filtering by shipwright group.
func hasShipwright(shpcli *shpwcli.Clientset) bool {
	list, err := shpcli.Discovery().ServerGroups()
	if err != nil {
		klog.Fatalf("error listing server groups: %v", err)
	}
	for _, group := range list.Groups {
		if group.Name != shpwv1alpha1.SchemeGroupVersion.Group {
			continue
		}
		klog.Infof("shipwright support enabled")
		return true
	}
	klog.Infof("shipwright support disabled")
	return false
}

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	coreinf "k8s.io/client-go/informers"
	corecli "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/ricardomaraschini/tagger/controllers"
	itagcli "github.com/ricardomaraschini/tagger/imagetags/generated/clientset/versioned"
	itaginf "github.com/ricardomaraschini/tagger/imagetags/generated/informers/externalversions"
	"github.com/ricardomaraschini/tagger/services"
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		cancel()
	}()

	klog.Info(` _|_  __,   __,  __,  _   ,_       `)
	klog.Info(`  |  /  |  /  | /  | |/  /  |      `)
	klog.Info(`  |_/\_/|_/\_/|/\_/|/|__/   |_/ .  `)
	klog.Info(`             /|   /|               `)
	klog.Info(`             \|   \|               `)
	klog.Info(`starting image tag controller...   `)

	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		klog.Fatalf("unable to read kubeconfig: %v", err)
	}

	// creates image tag client, informer and lister.
	tagcli, err := itagcli.NewForConfig(config)
	if err != nil {
		log.Fatalf("unable to create image tag client: %v", err)
	}
	taginf := itaginf.NewSharedInformerFactory(tagcli, time.Minute)
	taglis := taginf.Images().V1().Tags().Lister()

	// creates core client, informer and lister.
	corcli, err := corecli.NewForConfig(config)
	if err != nil {
		log.Fatalf("unable to create core client: %v", err)
	}
	corinf := coreinf.NewSharedInformerFactory(corcli, time.Minute)
	cnflis := corinf.Core().V1().ConfigMaps().Lister()
	seclis := corinf.Core().V1().Secrets().Lister()
	replis := corinf.Apps().V1().ReplicaSets().Lister()
	deplis := corinf.Apps().V1().Deployments().Lister()

	depsvc := services.NewDeployment(corcli, deplis, taglis)
	syssvc := services.NewSysContext(cnflis, seclis)
	impsvc := services.NewImporter(syssvc)
	tagsvc := services.NewTag(corcli, tagcli, taglis, replis, deplis, impsvc)
	itctrl := controllers.NewTag(taginf, tagsvc, 10)
	whctrl := controllers.NewWebHook(tagsvc)
	dpctrl := controllers.NewDeployment(corinf, depsvc)

	// starts up all informers and waits for their cache to sync
	// up, only then we start the operator i.e. start to process
	// events from the queue. XXX This is cumbersome as we don't
	// know exactly to which caches wait for sync, for now we hard
	// coded here Secrets, ReplicaSets, Deployments, ImageStreams
	// and ConfigMaps but later on this list may get way longer.
	klog.Info("waiting for caches to sync ...")
	corinf.Start(ctx.Done())
	taginf.Start(ctx.Done())
	if !cache.WaitForCacheSync(
		ctx.Done(),
		corinf.Core().V1().ConfigMaps().Informer().HasSynced,
		corinf.Core().V1().Secrets().Informer().HasSynced,
		corinf.Apps().V1().ReplicaSets().Informer().HasSynced,
		corinf.Apps().V1().Deployments().Informer().HasSynced,
		taginf.Images().V1().Tags().Informer().HasSynced,
	) {
		klog.Fatal("caches not syncing")
	}
	klog.Info("caches in sync, moving on.")

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		klog.Infof("starting webhooks handler")
		if err := whctrl.Start(ctx); err != nil {
			klog.Fatalf("http server error: %s", err)
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		klog.Infof("starting deployment controller")
		dpctrl.Start(ctx)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		klog.Infof("starting tags controller")
		itctrl.Start(ctx)
		wg.Done()
	}()

	wg.Wait()
}

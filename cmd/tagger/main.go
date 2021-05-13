package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	coreinf "k8s.io/client-go/informers"
	corecli "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/ricardomaraschini/tagger/controllers"
	itagcli "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/clientset/versioned"
	itaginf "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/informers/externalversions"
	"github.com/ricardomaraschini/tagger/services"
)

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

	klog.Info(` _|_  __,   __,  __,  _   ,_    `)
	klog.Info(`  |  /  |  /  | /  | |/  /  |   `)
	klog.Info(`  |_/\_/|_/\_/|/\_/|/|__/   |_/ `)
	klog.Info(`             /|   /|            `)
	klog.Info(`             \|   \|            `)
	klog.Info(`starting image tag controller...`)

	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		klog.Fatalf("unable to read kubeconfig: %v", err)
	}

	// creates tag client and informer.
	tagcli, err := itagcli.NewForConfig(config)
	if err != nil {
		log.Fatalf("unable to create image tag client: %v", err)
	}
	taginf := itaginf.NewSharedInformerFactory(tagcli, time.Minute)

	// creates core client and informer.
	corcli, err := corecli.NewForConfig(config)
	if err != nil {
		log.Fatalf("unable to create core client: %v", err)
	}
	corinf := coreinf.NewSharedInformerFactory(corcli, time.Minute)

	// create our service layer
	depsvc := services.NewDeployment(corcli, corinf, taginf)
	tagsvc := services.NewTag(corinf, tagcli, taginf)
	tiosvc := services.NewTagIO(corinf, tagcli, taginf)
	podsvc := services.NewPod(corcli, corinf)
	usrsvc := services.NewUser(corcli)
	mtrsvc := services.NewMetrics()

	// create controller layer
	itctrl := controllers.NewTag(tagsvc, mtrsvc)
	mtctrl := controllers.NewMutatingWebHook()
	qyctrl := controllers.NewQuayWebHook(tagsvc)
	dkctrl := controllers.NewDockerWebHook(tagsvc)
	dpctrl := controllers.NewDeployment(depsvc, tagsvc)
	tioctr := controllers.NewTagIO(tiosvc, usrsvc)
	pdctrl := controllers.NewPod(podsvc)
	moctrl := controllers.NewMetric()

	// starts up all informers and waits for their cache to sync up,
	// only then we start the controllers i.e. start to process events
	// from the queue.
	klog.Info("waiting for caches to sync ...")
	corinf.Start(ctx.Done())
	taginf.Start(ctx.Done())
	if !cache.WaitForCacheSync(
		ctx.Done(),
		corinf.Core().V1().ConfigMaps().Informer().HasSynced,
		corinf.Core().V1().Secrets().Informer().HasSynced,
		corinf.Core().V1().Pods().Informer().HasSynced,
		corinf.Apps().V1().Deployments().Informer().HasSynced,
		taginf.Images().V1beta1().Tags().Informer().HasSynced,
	) {
		klog.Fatal("caches not syncing")
	}
	klog.Info("caches in sync, moving on.")

	starter := NewStarter(
		corcli, mtctrl, qyctrl,
		dkctrl, dpctrl, itctrl,
		moctrl, tioctr, pdctrl,
	)
	if err := starter.Start(ctx); err != nil {
		klog.Errorf("unable to start controllers: %s", err)
	}
}

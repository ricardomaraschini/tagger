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

// Controller interface is implemented by all our controllers. Designs a
// process or thread that can be started with a context.
type Controller interface {
	Start(ctx context.Context) error
	Name() string
}

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
	tagsvc := services.NewTag(corcli, corinf, tagcli, taginf)
	mtrsvc := services.NewMetrics()

	// create controller layer
	itctrl := controllers.NewTag(tagsvc, mtrsvc)
	mtctrl := controllers.NewMutatingWebHook(tagsvc)
	qyctrl := controllers.NewQuayWebHook(tagsvc)
	dkctrl := controllers.NewDockerWebHook(tagsvc)
	dpctrl := controllers.NewDeployment(depsvc)
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
		corinf.Apps().V1().ReplicaSets().Informer().HasSynced,
		corinf.Apps().V1().Deployments().Informer().HasSynced,
		taginf.Images().V1().Tags().Informer().HasSynced,
	) {
		klog.Fatal("caches not syncing")
	}
	klog.Info("caches in sync, moving on.")

	var wg sync.WaitGroup
	ctrls := []Controller{mtctrl, qyctrl, dkctrl, dpctrl, itctrl, moctrl}
	for _, ctrl := range ctrls {
		wg.Add(1)
		go func(c Controller) {
			defer wg.Done()
			klog.Infof("starting controller for %q", c.Name())
			if err := c.Start(ctx); err != nil {
				klog.Errorf("%q failed: %s", c.Name(), err)
				return
			}
			klog.Infof("%q controller ended.", c.Name())
		}(ctrl)
	}
	wg.Wait()
}

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

	"github.com/ricardomaraschini/it/controllers"
	itagcli "github.com/ricardomaraschini/it/imagetags/generated/clientset/versioned"
	itaginf "github.com/ricardomaraschini/it/imagetags/generated/informers/externalversions"
	"github.com/ricardomaraschini/it/services"
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

	klog.Info(`     )               (    `)
	klog.Info(`    / \  .-"""""-.  / \   `)
	klog.Info(`   (   \/ __   __ \/   )  `)
	klog.Info(`    )  ; / _\ /_ \ ;  (   `)
	klog.Info(`   (   |  / \ / \  |   )  `)
	klog.Info(`    \ (,  \0/_\0/  ,) /   `)
	klog.Info(`     \_|   /   \   |_/    `)
	klog.Info(`       | (_\___/_) |      `)
	klog.Info(`       .\ \ -.- / /.      `)
	klog.Info(`      {  \ '===' /  }     `)
	klog.Info(`     {    '.___.'    }    `)
	klog.Info(`      {             }     `)
	klog.Info(`       '"="="="="="'      `)
	klog.Info("starting image tag controller...")

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
	seclis := corinf.Core().V1().Secrets().Lister()
	replis := corinf.Apps().V1().ReplicaSets().Lister()
	deplis := corinf.Apps().V1().Deployments().Lister()

	syssvc := services.NewSysContext(seclis)
	impsvc := services.NewImporter(syssvc)
	tagsvc := services.NewTag(corcli, tagcli, taglis, replis, deplis, impsvc)
	itctrl := controllers.NewTag(taginf, tagsvc, 10)
	whctrl := controllers.NewWebHook(tagsvc)

	klog.Info("waiting for caches to sync ...")
	// starts up all informers and waits for their cache to sync
	// up, only then we start the operator i.e. start to process
	// events from the queue. XXX This is cumbersome as we don't
	// know exactly to which caches wait for sync, for now we hard
	// coded here Secrets, ReplicaSets, Deployments and ImageStreams
	// but later on this list may get way longer.
	corinf.Start(ctx.Done())
	taginf.Start(ctx.Done())
	if !cache.WaitForCacheSync(
		ctx.Done(),
		corinf.Core().V1().Secrets().Informer().HasSynced,
		corinf.Apps().V1().ReplicaSets().Informer().HasSynced,
		corinf.Apps().V1().Deployments().Informer().HasSynced,
		taginf.Images().V1().Tags().Informer().HasSynced,
	) {
		klog.Fatal("caches not syncing")
	}
	klog.Info("caches synced.")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		if err := whctrl.Start(ctx); err != nil {
			klog.Fatalf("http server error: %s", err)
		}
		wg.Done()
	}()

	klog.Infof("starting to process queue events")
	itctrl.Start(ctx)
	wg.Wait()
}

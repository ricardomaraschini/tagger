package main

// XXX REFACTOR ME COMPLETELY

import (
	"context"
	"fmt"
	"log"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	itagcli "github.com/ricardomaraschini/tagger/imagetags/generated/clientset/versioned"
)

func help() {
	fmt.Println("help message")
	os.Exit(1)
}

func downgradeTag(ctx context.Context, cli itagcli.Interface, name string) {
	tag, err := cli.ImagesV1().Tags("default").Get(
		ctx, name, metav1.GetOptions{},
	)
	if err != nil {
		klog.Fatal(err)
	}

	expectedGen := tag.Spec.Generation - 1
	found := false
	for _, ref := range tag.Status.References {
		if ref.Generation != expectedGen {
			continue
		}
		found = true
		break
	}

	if !found {
		klog.Error("you are on the oldest generation")
		return
	}

	tag.Spec.Generation = expectedGen
	tag, err = cli.ImagesV1().Tags("default").Update(
		ctx, tag, metav1.UpdateOptions{},
	)
	if err != nil {
		panic(err)
	}

	klog.Infof("tag %s downgraded to generation %d", name, tag.Spec.Generation)
}

func upgradeTag(ctx context.Context, cli itagcli.Interface, name string) {
	tag, err := cli.ImagesV1().Tags("default").Get(
		ctx, name, metav1.GetOptions{},
	)
	if err != nil {
		klog.Fatal(err)
	}

	tag.Spec.Generation++

	tag, err = cli.ImagesV1().Tags("default").Update(
		ctx, tag, metav1.UpdateOptions{},
	)
	if err != nil {
		panic(err)
	}

	klog.Infof("tag %s upgrade to generation %d", name, tag.Spec.Generation)
}

func importTag(ctx context.Context, cli itagcli.Interface, name string) {
	tag, err := cli.ImagesV1().Tags("default").Get(
		ctx, name, metav1.GetOptions{},
	)
	if err != nil {
		klog.Fatal(err)
	}

	nextGen := int64(0)
	if len(tag.Status.References) > 0 {
		nextGen = tag.Status.References[0].Generation + 1
	}
	tag.Spec.Generation = nextGen

	tag, err = cli.ImagesV1().Tags("default").Update(
		ctx, tag, metav1.UpdateOptions{},
	)
	if err != nil {
		panic(err)
	}

	klog.Infof("tag %s import process started, generation %d", name, tag.Spec.Generation)
}

func main() {
	ctx := context.Background()
	if len(os.Args) != 3 {
		help()
	}

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

	switch os.Args[1] {
	case "upgrade":
		upgradeTag(ctx, tagcli, os.Args[2])
	case "downgrade":
		downgradeTag(ctx, tagcli, os.Args[2])
	case "import":
		importTag(ctx, tagcli, os.Args[2])
	default:
		help()
	}
}

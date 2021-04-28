package services

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	corecli "k8s.io/client-go/kubernetes"
	corelis "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// Pod gather all actions related to pod objects.
type Pod struct {
	corcli corecli.Interface
	corinf informers.SharedInformerFactory
	corlis corelis.PodLister
}

// NewPod returns a handler for all pod related services.
func NewPod(corcli corecli.Interface, corinf informers.SharedInformerFactory) *Pod {
	var corlis corelis.PodLister
	if corinf != nil {
		corlis = corinf.Core().V1().Pods().Lister()
	}
	return &Pod{
		corcli: corcli,
		corinf: corinf,
		corlis: corlis,
	}
}

// Sync verifies if the provided pod uses a tag, if it does use a tag we update its
// image reference to point to the current version of the tag. By syncing a pod we
// mean: copy tag references present in pod annotations and put them in the image
// property of the containers.
func (p *Pod) Sync(ctx context.Context, pod *corev1.Pod) error {
	if _, ok := pod.Annotations["image-tag"]; !ok {
		return nil
	}

	changed := false
	for i, c := range pod.Spec.Containers {
		ref, ok := pod.Annotations[c.Image]
		if !ok || c.Image == ref {
			continue
		}
		changed = true
		pod.Spec.Containers[i].Image = ref
	}

	for i, c := range pod.Spec.InitContainers {
		ref, ok := pod.Annotations[c.Image]
		if !ok || c.Image == ref {
			continue
		}
		changed = true
		pod.Spec.InitContainers[i].Image = ref
	}

	if !changed {
		return nil
	}

	_, err := p.corcli.CoreV1().Pods(pod.Namespace).Update(
		ctx, pod, metav1.UpdateOptions{},
	)
	return err
}

// Get returns a pod by namespace/name pair.
func (p *Pod) Get(ctx context.Context, ns, name string) (*corev1.Pod, error) {
	pod, err := p.corlis.Pods(ns).Get(name)
	if err != nil {
		return nil, fmt.Errorf("unable get pod: %w", err)
	}
	return pod.DeepCopy(), nil
}

// AddEventHandler adds provided handler to Pod related events.
func (p *Pod) AddEventHandler(handler cache.ResourceEventHandler) {
	p.corinf.Core().V1().Pods().Informer().AddEventHandler(handler)
}

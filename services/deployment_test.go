package services

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	coreinf "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	imagtagv1beta1 "github.com/ricardomaraschini/tagger/infra/tags/v1beta1"
	tagfake "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/clientset/versioned/fake"
	itaginf "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/informers/externalversions"
)

func TestDeploymentsForTag(t *testing.T) {
	for _, tt := range []struct {
		name    string
		objects []runtime.Object
		tag     *imagtagv1beta1.Tag
		deploys []string
	}{
		{
			name:    "no deploys",
			deploys: []string{},
			tag:     &imagtagv1beta1.Tag{},
		},
		{
			name:    "deploy using a different tag",
			deploys: []string{},
			tag: &imagtagv1beta1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myimage",
					Namespace: "ns",
				},
			},
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment0",
						Namespace: "ns",
						Annotations: map[string]string{
							"image-tag": "true",
						},
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Image: "tag0",
									},
								},
							},
						},
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment1",
						Namespace: "ns",
						Annotations: map[string]string{
							"image-tag": "true",
						},
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Image: "tag1",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:    "two deploys",
			deploys: []string{"ns/deployment0", "ns/deployment1"},
			tag: &imagtagv1beta1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myimage",
					Namespace: "ns",
				},
			},
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment0",
						Namespace: "ns",
						Annotations: map[string]string{
							"image-tag": "true",
						},
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Image: "myimage",
									},
								},
							},
						},
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment1",
						Namespace: "ns",
						Annotations: map[string]string{
							"image-tag": "true",
						},
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Image: "myimage",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:    "one deploy",
			deploys: []string{"ns/deployment"},
			tag: &imagtagv1beta1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myimage",
					Namespace: "ns",
				},
			},
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment",
						Namespace: "ns",
						Annotations: map[string]string{
							"image-tag": "true",
						},
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Image: "myimage",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:    "deploy using tag but without annotation",
			deploys: []string{},
			tag: &imagtagv1beta1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myimage",
					Namespace: "ns",
				},
			},
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment",
						Namespace: "ns",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Image: "myimage",
									},
								},
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			fakecli := fake.NewSimpleClientset(tt.objects...)
			informer := coreinf.NewSharedInformerFactory(fakecli, time.Minute)
			deplis := informer.Apps().V1().Deployments().Lister()

			informer.Start(ctx.Done())
			if !cache.WaitForCacheSync(
				ctx.Done(),
				informer.Apps().V1().Deployments().Informer().HasSynced,
			) {
				t.Fatal("errors waiting for caches to sync")
			}

			svc := Deployment{
				deplis: deplis,
			}

			deps, err := svc.DeploymentsForTag(ctx, tt.tag)
			if err != nil {
				t.Errorf("error should be nil, not %q", err.Error())
			}

			depnames := make([]string, len(deps))
			for i, dep := range deps {
				depnames[i] = fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
			}
			if len(depnames) != len(tt.deploys) {
				t.Errorf(
					"expecting %d deploys, %d returned",
					len(tt.deploys), len(depnames),
				)
			}
			for _, exp := range tt.deploys {
				found := false
				for _, d := range depnames {
					if exp != d {
						continue
					}
					found = true
					break
				}
				if !found {
					t.Errorf("expected to find %s in %+v", exp, depnames)
				}
			}
		})
	}
}

func TestDeploymentSync(t *testing.T) {
	for _, tt := range []struct {
		name   string
		deploy *appsv1.Deployment
		tags   []runtime.Object
		exp    map[string]string
	}{
		{
			name: "tag not imported yet",
			exp: map[string]string{
				"image-tag": "true",
			},
			deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mydeploy",
					Namespace: "ns",
					Annotations: map[string]string{
						"image-tag": "true",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "mytag",
								},
							},
						},
					},
				},
			},
			tags: []runtime.Object{
				&imagtagv1beta1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mytag",
						Namespace: "ns",
					},
					Status: imagtagv1beta1.TagStatus{
						Generation: 3,
						References: []imagtagv1beta1.HashReference{
							{
								Generation:     2,
								ImageReference: "remoteimage:123",
							},
							{
								Generation:     1,
								ImageReference: "remoteimage:321",
							},
						},
					},
				},
			},
		},
		{
			name: "tag not found",
			exp: map[string]string{
				"image-tag": "true",
			},
			deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mydeploy",
					Namespace: "ns",
					Annotations: map[string]string{
						"image-tag": "true",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "mytag",
								},
							},
						},
					},
				},
			},
			tags: []runtime.Object{
				&imagtagv1beta1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "anothertag",
						Namespace: "ns",
					},
					Status: imagtagv1beta1.TagStatus{
						Generation: 2,
						References: []imagtagv1beta1.HashReference{
							{
								Generation:     2,
								ImageReference: "remoteimage:123",
							},
							{
								Generation:     1,
								ImageReference: "remoteimage:321",
							},
						},
					},
				},
			},
		},
		{
			name: "deployment already tagged with annotation",
			exp: map[string]string{
				"image-tag": "true",
				"mytag":     "remoteimage:123",
			},
			deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mydeploy",
					Namespace: "ns",
					Annotations: map[string]string{
						"image-tag": "true",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"mytag": "remoteimage:321",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "mytag",
								},
							},
						},
					},
				},
			},
			tags: []runtime.Object{
				&imagtagv1beta1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mytag",
						Namespace: "ns",
					},
					Status: imagtagv1beta1.TagStatus{
						Generation: 2,
						References: []imagtagv1beta1.HashReference{
							{
								Generation:     2,
								ImageReference: "remoteimage:123",
							},
							{
								Generation:     1,
								ImageReference: "remoteimage:321",
							},
						},
					},
				},
			},
		},
		{
			name: "deployment using tag",
			exp: map[string]string{
				"mytag":     "remoteimage:123",
				"image-tag": "true",
			},
			deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mydeploy",
					Namespace: "ns",
					Annotations: map[string]string{
						"image-tag": "true",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "mytag",
								},
							},
						},
					},
				},
			},
			tags: []runtime.Object{
				&imagtagv1beta1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mytag",
						Namespace: "ns",
					},
					Status: imagtagv1beta1.TagStatus{
						Generation: 2,
						References: []imagtagv1beta1.HashReference{
							{
								Generation:     2,
								ImageReference: "remoteimage:123",
							},
							{
								Generation:     1,
								ImageReference: "remoteimage:321",
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			corcli := fake.NewSimpleClientset(tt.deploy)
			fakecli := tagfake.NewSimpleClientset(tt.tags...)
			taginf := itaginf.NewSharedInformerFactory(fakecli, time.Minute)
			taglis := taginf.Tagger().V1beta1().Tags().Lister()

			taginf.Start(ctx.Done())
			if !cache.WaitForCacheSync(
				ctx.Done(),
				taginf.Tagger().V1beta1().Tags().Informer().HasSynced,
			) {
				t.Fatal("errors waiting for caches to sync")
			}

			svc := Deployment{
				taglis: taglis,
				corcli: corcli,
			}

			if err := svc.Sync(ctx, tt.deploy); err != nil {
				t.Errorf("error should be nil, not %q", err.Error())
			}

			deploy, err := corcli.AppsV1().Deployments(tt.deploy.Namespace).Get(
				ctx, tt.deploy.Name, metav1.GetOptions{},
			)
			if err != nil {
				t.Errorf("unexpected error fetching deployment: %s", err)
			}

			if !reflect.DeepEqual(deploy.Spec.Template.Annotations, tt.exp) {
				t.Errorf(
					"expected annotations to be %+v, they are %+v instead",
					tt.exp, deploy.Spec.Template.Annotations,
				)
			}
		})
	}
}

package services

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mattbaird/jsonpatch"
	imagtagv1 "github.com/ricardomaraschini/it/imagetags/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	coreinf "k8s.io/client-go/informers"
	corfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	tagfake "github.com/ricardomaraschini/it/imagetags/generated/clientset/versioned/fake"
	taginf "github.com/ricardomaraschini/it/imagetags/generated/informers/externalversions"
)

func TestValidateTagGeneration(t *testing.T) {
	for _, tt := range []struct {
		name string
		tag  imagtagv1.Tag
		err  string
	}{
		{
			name: "empty tag",
			tag:  imagtagv1.Tag{},
		},
		{
			name: "invalid generation",
			err:  "generation must be one of: [0]",
			tag: imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					Generation: 2,
				},
			},
		},
		{
			name: "next generation",
			tag: imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					Generation: 10,
				},
				Status: imagtagv1.TagStatus{
					References: []imagtagv1.HashReference{
						{Generation: 9},
						{Generation: 8},
					},
				},
			},
		},
		{
			name: "one old but valid generation",
			tag: imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					Generation: 2,
				},
				Status: imagtagv1.TagStatus{
					References: []imagtagv1.HashReference{
						{Generation: 5},
						{Generation: 4},
						{Generation: 3},
						{Generation: 2},
					},
				},
			},
		},
		{
			name: "negative generation",
			err:  "generation must be one of: [0]",
			tag: imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					Generation: -1,
				},
				Status: imagtagv1.TagStatus{
					References: nil,
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewTag(nil, nil, nil, nil, nil, nil)
			err := svc.ValidateTagGeneration(tt.tag)
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error %s", err)
					return
				}
				if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("invalid error %s", err)
				}
				return
			} else if len(tt.err) > 0 {
				t.Errorf("expecting %q, nil received instead", tt.err)
			}
		})
	}
}

func TestCurrentReferenceForTag(t *testing.T) {
	for _, tt := range []struct {
		name    string
		itname  string
		objects []runtime.Object
		expref  string
		err     string
	}{
		{
			name: "no image tag",
		},
		{
			name:   "generation not existent",
			err:    "generation does not exist",
			itname: "tag",
			objects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tag",
						Namespace: "default",
					},
				},
			},
		},
		{
			name:   "happy path",
			expref: "my ref",
			itname: "tag",
			objects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tag",
						Namespace: "default",
					},
					Status: imagtagv1.TagStatus{
						Generation: 60,
						References: []imagtagv1.HashReference{
							{Generation: 63},
							{Generation: 62},
							{Generation: 61},
							{
								Generation:     60,
								ImageReference: "my ref",
							},
						},
					},
				},
			},
		},
		{
			name:   "tag in different namespace",
			itname: "tag",
			objects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tag",
						Namespace: "another-namespace",
					},
					Status: imagtagv1.TagStatus{
						Generation: 0,
						References: []imagtagv1.HashReference{
							{
								Generation:     0,
								ImageReference: "ref",
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

			fakecli := tagfake.NewSimpleClientset(tt.objects...)
			informer := taginf.NewSharedInformerFactory(fakecli, time.Minute)
			taglis := informer.Images().V1().Tags().Lister()
			informer.Start(ctx.Done())
			if !cache.WaitForCacheSync(
				ctx.Done(),
				informer.Images().V1().Tags().Informer().HasSynced,
			) {
				t.Fatal("errors waiting for caches to sync")
			}

			svc := NewTag(nil, nil, taglis, nil, nil, nil)
			ref, err := svc.CurrentReferenceForTag("default", tt.itname)
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error: %s", err)
				} else if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("expecting %q, %q received instead", tt.err, err)
				}
			} else if len(tt.err) > 0 {
				t.Errorf("expecting error %q, nil received instead", tt.err)
			}

			if ref != tt.expref {
				t.Errorf("expecting ref %q, received %q instead", tt.expref, ref)
			}
		})
	}
}

func TestPatchForDeployment(t *testing.T) {
	for _, tt := range []struct {
		name     string
		deploy   appsv1.Deployment
		objects  []runtime.Object
		expected []jsonpatch.JsonPatchOperation
		err      string
	}{
		{
			name:   "deployment without annotation",
			deploy: appsv1.Deployment{},
		},
		{
			name: "happy path",
			expected: []jsonpatch.JsonPatchOperation{
				{
					Operation: "add",
					Path:      "/spec/template/metadata/annotations",
					Value: map[string]interface{}{
						"imagetag": "1",
					},
				},
			},
			deploy: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-deploy",
					Annotations: map[string]string{
						"image-tag": "true",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "imagetag",
								},
							},
						},
					},
				},
			},
			objects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "imagetag",
						Namespace: "default",
					},
					Status: imagtagv1.TagStatus{
						Generation: 1,
						References: []imagtagv1.HashReference{
							{Generation: 2},
							{
								Generation:     1,
								ImageReference: "my ref",
							},
						},
					},
				},
			},
		},
		{
			name: "happy path with multiple pods",
			expected: []jsonpatch.JsonPatchOperation{
				{
					Operation: "add",
					Path:      "/spec/template/metadata/annotations",
					Value: map[string]interface{}{
						"imagetag":   "1",
						"anothertag": "2",
					},
				},
			},
			deploy: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-deploy",
					Annotations: map[string]string{
						"image-tag": "true",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Image: "imagetag"},
								{Image: "anothertag"},
							},
						},
					},
				},
			},
			objects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "imagetag",
						Namespace: "default",
					},
					Status: imagtagv1.TagStatus{
						Generation: 1,
						References: []imagtagv1.HashReference{
							{Generation: 2},
							{
								Generation:     1,
								ImageReference: "my ref",
							},
						},
					},
				},
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "anothertag",
						Namespace: "default",
					},
					Status: imagtagv1.TagStatus{
						Generation: 2,
						References: []imagtagv1.HashReference{
							{
								Generation:     2,
								ImageReference: "another ref",
							},
							{
								Generation:     1,
								ImageReference: "my ref",
							},
						},
					},
				},
			},
		},
		{
			name: "tag not imported yet",
			deploy: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-deploy",
					Annotations: map[string]string{
						"image-tag": "true",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "imagetag",
								},
							},
						},
					},
				},
			},
			objects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "imagetag",
						Namespace: "default",
					},
					Status: imagtagv1.TagStatus{
						Generation: 1,
						References: []imagtagv1.HashReference{
							{
								Generation: 0,
							},
						},
					},
				},
			},
		},
		{
			name: "non existent image tag",
			deploy: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-deploy",
					Annotations: map[string]string{
						"image-tag": "true",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "non-tag",
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

			fakecli := tagfake.NewSimpleClientset(tt.objects...)
			informer := taginf.NewSharedInformerFactory(fakecli, time.Minute)
			taglis := informer.Images().V1().Tags().Lister()
			informer.Start(ctx.Done())
			if !cache.WaitForCacheSync(
				ctx.Done(),
				informer.Images().V1().Tags().Informer().HasSynced,
			) {
				t.Fatal("errors waiting for caches to sync")
			}

			svc := NewTag(nil, nil, taglis, nil, nil, nil)
			patch, err := svc.PatchForDeployment(tt.deploy)
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error: %s", err)
				} else if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("expecting %q, %q received instead", tt.err, err)
				}
			} else if len(tt.err) > 0 {
				t.Errorf("expecting error %q, nil received instead", tt.err)
			}

			if !reflect.DeepEqual(tt.expected, patch) {
				t.Errorf("patch mismatch: %v, %v", tt.expected, patch)
			}
		})
	}
}

func TestPatchForPod(t *testing.T) {
	for _, tt := range []struct {
		name     string
		pod      corev1.Pod
		tags     []runtime.Object
		replicas []runtime.Object
		expected []jsonpatch.JsonPatchOperation
		err      string
	}{
		{
			name: "pod without owner",
			pod:  corev1.Pod{},
		},
		{
			name: "job pod",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-pod",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "Job",
						},
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "imagetag",
						},
					},
				},
			},
			tags: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "imagetag",
						Namespace: "default",
					},
					Status: imagtagv1.TagStatus{
						Generation: 1,
						References: []imagtagv1.HashReference{
							{
								Generation: 0,
							},
						},
					},
				},
			},
		},
		{
			name: "happy path",
			expected: []jsonpatch.JsonPatchOperation{
				{
					Operation: "replace",
					Path:      "/spec/containers/0/image",
					Value:     "image ref",
				},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-pod",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ReplicaSet",
							Name: "replicaset",
						},
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "imagetag",
						},
					},
				},
			},
			tags: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "imagetag",
						Namespace: "default",
					},
					Status: imagtagv1.TagStatus{
						Generation: 0,
						References: []imagtagv1.HashReference{
							{
								Generation:     0,
								ImageReference: "image ref",
							},
						},
					},
				},
			},
			replicas: []runtime.Object{
				&appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "replicaset",
						Namespace: "default",
						Annotations: map[string]string{
							"image-tag": "true",
						},
					},
				},
			},
		},
		{
			name: "replica without annotation",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-pod",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ReplicaSet",
							Name: "replicaset",
						},
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "imagetag",
						},
					},
				},
			},
			replicas: []runtime.Object{
				&appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "replicaset",
						Namespace: "default",
					},
				},
			},
		},
		{
			name: "non existing replica set",
			err:  "not found",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-pod",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ReplicaSet",
							Name: "replicaset",
						},
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "imagetag",
						},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			tagcli := tagfake.NewSimpleClientset(tt.tags...)
			taginf := taginf.NewSharedInformerFactory(tagcli, time.Minute)
			taglis := taginf.Images().V1().Tags().Lister()

			corcli := corfake.NewSimpleClientset(tt.replicas...)
			corinf := coreinf.NewSharedInformerFactory(corcli, time.Minute)
			rslist := corinf.Apps().V1().ReplicaSets().Lister()

			taginf.Start(ctx.Done())
			corinf.Start(ctx.Done())
			if !cache.WaitForCacheSync(
				ctx.Done(),
				taginf.Images().V1().Tags().Informer().HasSynced,
				corinf.Apps().V1().ReplicaSets().Informer().HasSynced,
			) {
				t.Fatal("errors waiting for caches to sync")
			}

			svc := NewTag(nil, nil, taglis, rslist, nil, nil)
			patch, err := svc.PatchForPod(tt.pod)
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error: %s", err)
				} else if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("expecting %q, %q received instead", tt.err, err)
				}
			} else if len(tt.err) > 0 {
				t.Errorf("expecting error %q, nil received instead", tt.err)
			}

			if !reflect.DeepEqual(tt.expected, patch) {
				t.Errorf("patch mismatch: %v, %v", tt.expected, patch)
			}
		})
	}
}

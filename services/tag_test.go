package services

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	coreinf "k8s.io/client-go/informers"
	corfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/mattbaird/jsonpatch"

	imagtagv1 "github.com/ricardomaraschini/tagger/infra/tags/v1"
	tagfake "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/clientset/versioned/fake"
	taginf "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/informers/externalversions"
)

func TestPatchForPod(t *testing.T) {
	for _, tt := range []struct {
		name     string
		pod      corev1.Pod
		expected []jsonpatch.JsonPatchOperation
		err      string
	}{
		{
			name: "pod without annotation",
			pod:  corev1.Pod{},
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
					Annotations: map[string]string{
						"image-tag": "true",
						"imagetag":  "image ref",
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
		{
			name: "pod without image tag annotation yet",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-pod",
					Annotations: map[string]string{
						"image-tag": "true",
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
			svc := &Tag{}
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

func TestSync(t *testing.T) {
	for _, tt := range []struct {
		name       string
		tag        *imagtagv1.Tag
		err        string
		corObjects []runtime.Object
		tagObjects []runtime.Object
		succeed    bool
		importErr  string
	}{
		{
			name:    "empty tag",
			err:     "empty tag reference",
			succeed: false,
			tag: &imagtagv1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "empty-tag",
				},
			},
		},
		{
			name:    "import of non existing tag",
			err:     "manifest unknown",
			succeed: false,
			tag: &imagtagv1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "new-tag",
				},
				Spec: imagtagv1.TagSpec{
					From:       "centos:xyz123xyz",
					Generation: 0,
				},
			},
		},
		{
			name:    "first import (happy path)",
			succeed: true,
			tag: &imagtagv1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "new-tag",
				},
				Spec: imagtagv1.TagSpec{
					From:       "centos:latest",
					Generation: 0,
				},
			},
			tagObjects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "new-tag",
					},
					Spec: imagtagv1.TagSpec{
						From:       "centos:latest",
						Generation: 0,
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			corcli := corfake.NewSimpleClientset(tt.corObjects...)
			corinf := coreinf.NewSharedInformerFactory(corcli, time.Minute)

			tagcli := tagfake.NewSimpleClientset(tt.tagObjects...)
			taginf := taginf.NewSharedInformerFactory(tagcli, time.Minute)

			svc := NewTag(corcli, corinf, tagcli, taginf)

			corinf.Start(ctx.Done())
			taginf.Start(ctx.Done())
			if !cache.WaitForCacheSync(
				ctx.Done(),
				corinf.Core().V1().ConfigMaps().Informer().HasSynced,
				corinf.Core().V1().Secrets().Informer().HasSynced,
				corinf.Apps().V1().Deployments().Informer().HasSynced,
				taginf.Images().V1().Tags().Informer().HasSynced,
			) {
				t.Fatal("errors waiting for caches to sync")
			}

			err := svc.Sync(ctx, tt.tag)
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error: %s", err)
				} else if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("expecting %q, %q received instead", tt.err, err)
				}
			} else if len(tt.err) > 0 {
				t.Errorf("expecting error %q, nil received instead", tt.err)
			}

			succeed := tt.tag.Status.LastImportAttempt.Succeed
			if succeed != tt.succeed {
				t.Errorf("wrong succeed(%v), received %v", tt.succeed, succeed)
			}

			if !tt.succeed {
				reason := tt.tag.Status.LastImportAttempt.Reason
				if !strings.Contains(reason, tt.err) {
					t.Errorf("unexpected failure reason: %q", reason)
				}
			}

		})
	}
}

func TestNewGenerationForImageRef(t *testing.T) {
	for _, tt := range []struct {
		name       string
		imgpath    string
		expgens    []int64
		err        string
		tagObjects []runtime.Object
	}{
		{
			name:    "no tags",
			imgpath: "quay.io/repo/image:latest",
		},
		{
			name:    "tag not imported yet",
			imgpath: "quay.io/repo/image:latest",
			expgens: []int64{2},
			tagObjects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "name",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 2,
						From:       "quay.io/repo/image:latest",
					},
				},
			},
		},
		{
			name:    "happy path",
			imgpath: "quay.io/repo/image:latest",
			expgens: []int64{3},
			tagObjects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "name",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 2,
						From:       "quay.io/repo/image:latest",
					},
					Status: imagtagv1.TagStatus{
						References: []imagtagv1.HashReference{
							{
								Generation: 2,
							},
						},
					},
				},
			},
		},
		{
			name:    "tags in different namespaces",
			imgpath: "quay.io/repo/image:latest",
			expgens: []int64{3, 4},
			tagObjects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "a_namespace",
						Name:      "a_name",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 2,
						From:       "quay.io/repo/image:latest",
					},
					Status: imagtagv1.TagStatus{
						References: []imagtagv1.HashReference{
							{
								Generation: 2,
							},
						},
					},
				},
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "b_namespace",
						Name:      "b_name",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 3,
						From:       "quay.io/repo/image:latest",
					},
					Status: imagtagv1.TagStatus{
						References: []imagtagv1.HashReference{
							{
								Generation: 3,
							},
						},
					},
				},
			},
		},
		{
			name:    "tag not using imgpath",
			imgpath: "quay.io/repo/image:latest",
			expgens: []int64{2, 4},
			tagObjects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "a_namespace",
						Name:      "a_name",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 2,
						From:       "quay.io/repo2/image:latest",
					},
					Status: imagtagv1.TagStatus{
						References: []imagtagv1.HashReference{
							{
								Generation: 2,
							},
						},
					},
				},
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "b_namespace",
						Name:      "b_name",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 3,
						From:       "quay.io/repo/image:latest",
					},
					Status: imagtagv1.TagStatus{
						References: []imagtagv1.HashReference{
							{
								Generation: 3,
							},
						},
					},
				},
			},
		},
		{
			name:    "tag generation not imported yet",
			imgpath: "quay.io/repo/image:latest",
			expgens: []int64{2},
			tagObjects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "a_namespace",
						Name:      "a_name",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 2,
						From:       "quay.io/repo/image:latest",
					},
					Status: imagtagv1.TagStatus{
						References: []imagtagv1.HashReference{
							{
								Generation: 1,
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			tagcli := tagfake.NewSimpleClientset(tt.tagObjects...)
			taginf := taginf.NewSharedInformerFactory(tagcli, time.Minute)
			taglis := taginf.Images().V1().Tags().Lister()

			taginf.Start(ctx.Done())
			if !cache.WaitForCacheSync(
				ctx.Done(),
				taginf.Images().V1().Tags().Informer().HasSynced,
			) {
				t.Fatal("errors waiting for caches to sync")
			}

			tag := &Tag{
				tagcli: tagcli,
				taglis: taglis,
			}
			err := tag.NewGenerationForImageRef(ctx, tt.imgpath)
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error: %s", err)
				} else if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("expecting %q, %q received instead", tt.err, err)
				}
			} else if len(tt.err) > 0 {
				t.Errorf("expecting error %q, nil received instead", tt.err)
			}

			for i, obj := range tt.tagObjects {
				tag := obj.(*imagtagv1.Tag)
				if tag, err = tagcli.ImagesV1().Tags(tag.Namespace).Get(
					ctx, tag.Name, metav1.GetOptions{},
				); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if !reflect.DeepEqual(tag.Spec.Generation, tt.expgens[i]) {
					t.Errorf("unexpected gen for %+v", tag)
				}
			}
		})
	}
}

func TestUpgrade(t *testing.T) {
	for _, tt := range []struct {
		name         string
		tagName      string
		tagNamespace string
		expgen       int64
		err          string
		tagObjects   []runtime.Object
	}{
		{
			name:         "no tags",
			tagName:      "atag",
			tagNamespace: "atagnamespace",
			err:          "not found",
		},
		{
			name:         "tag pending import",
			tagName:      "atag",
			tagNamespace: "atagnamespace",
			err:          "pending tag import",
			tagObjects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "atag",
						Namespace: "atagnamespace",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 2,
					},
				},
			},
		},
		{
			name:         "happy path",
			tagName:      "atag",
			tagNamespace: "atagnamespace",
			expgen:       3,
			tagObjects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "atag",
						Namespace: "atagnamespace",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 2,
					},
					Status: imagtagv1.TagStatus{
						References: []imagtagv1.HashReference{
							{
								Generation: 2,
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			tagcli := tagfake.NewSimpleClientset(tt.tagObjects...)

			svc := &Tag{tagcli: tagcli}
			it, err := svc.Upgrade(ctx, tt.tagNamespace, tt.tagName)
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error: %s", err)
				} else if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("expecting %q, %q received instead", tt.err, err)
				}
			} else if len(tt.err) > 0 {
				t.Errorf("expecting error %q, nil received instead", tt.err)
			}

			if len(tt.err) == 0 {
				if it.Spec.Generation != tt.expgen {
					t.Errorf("unexpected gen: %v", it.Spec.Generation)
				}
			}
		})
	}
}

func TestDowngrade(t *testing.T) {
	for _, tt := range []struct {
		name         string
		tagName      string
		tagNamespace string
		expgen       int64
		err          string
		tagObjects   []runtime.Object
	}{
		{
			name:         "no tags",
			tagName:      "atag",
			tagNamespace: "atagnamespace",
			err:          "not found",
		},
		{
			name:         "oldest generation",
			tagName:      "atag",
			tagNamespace: "atagnamespace",
			err:          "at oldest generation",
			tagObjects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "atag",
						Namespace: "atagnamespace",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 2,
					},
				},
			},
		},
		{
			name:         "happy path",
			tagName:      "atag",
			tagNamespace: "atagnamespace",
			expgen:       1,
			tagObjects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "atag",
						Namespace: "atagnamespace",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 2,
					},
					Status: imagtagv1.TagStatus{
						References: []imagtagv1.HashReference{
							{
								Generation: 2,
							},
							{
								Generation: 1,
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			tagcli := tagfake.NewSimpleClientset(tt.tagObjects...)

			svc := &Tag{tagcli: tagcli}
			it, err := svc.Downgrade(ctx, tt.tagNamespace, tt.tagName)
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error: %s", err)
				} else if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("expecting %q, %q received instead", tt.err, err)
				}
			} else if len(tt.err) > 0 {
				t.Errorf("expecting error %q, nil received instead", tt.err)
			}

			if len(tt.err) == 0 {
				if it.Spec.Generation != tt.expgen {
					t.Errorf("unexpected gen: %v", it.Spec.Generation)
				}
			}
		})
	}
}

func TestNewGeneration(t *testing.T) {
	for _, tt := range []struct {
		name         string
		tagName      string
		tagNamespace string
		expgen       int64
		err          string
		tagObjects   []runtime.Object
	}{
		{
			name:         "no tags",
			tagName:      "atag",
			tagNamespace: "atagnamespace",
			err:          "not found",
		},
		{
			name:         "not imported yet",
			tagName:      "atag",
			tagNamespace: "atagnamespace",
			expgen:       0,
			tagObjects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "atag",
						Namespace: "atagnamespace",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 0,
					},
				},
			},
		},
		{
			name:         "happy path",
			tagName:      "atag",
			tagNamespace: "atagnamespace",
			expgen:       3,
			tagObjects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "atag",
						Namespace: "atagnamespace",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 2,
					},
					Status: imagtagv1.TagStatus{
						References: []imagtagv1.HashReference{
							{
								Generation: 2,
							},
							{
								Generation: 1,
							},
						},
					},
				},
			},
		},
		{
			name:         "fast forward",
			tagName:      "atag",
			tagNamespace: "atagnamespace",
			expgen:       5,
			tagObjects: []runtime.Object{
				&imagtagv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "atag",
						Namespace: "atagnamespace",
					},
					Spec: imagtagv1.TagSpec{
						Generation: 1,
					},
					Status: imagtagv1.TagStatus{
						Generation: 1,
						References: []imagtagv1.HashReference{
							{
								Generation: 4,
							},
							{
								Generation: 3,
							},
							{
								Generation: 2,
							},
							{
								Generation: 1,
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			tagcli := tagfake.NewSimpleClientset(tt.tagObjects...)

			svc := &Tag{tagcli: tagcli}
			it, err := svc.NewGeneration(ctx, tt.tagNamespace, tt.tagName)
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error: %s", err)
				} else if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("expecting %q, %q received instead", tt.err, err)
				}
			} else if len(tt.err) > 0 {
				t.Errorf("expecting error %q, nil received instead", tt.err)
			}

			if len(tt.err) == 0 {
				if it.Spec.Generation != tt.expgen {
					t.Errorf("unexpected gen: %v", it.Spec.Generation)
				}
			}
		})
	}
}

func Test_splitRegistryDomain(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		reg   string
		img   string
	}{
		{
			name:  "docker.io with explicit registry",
			input: "docker.io/centos:latest",
			reg:   "docker.io",
			img:   "centos:latest",
		},
		{
			name:  "docker.io without explicit registry",
			input: "centos:latest",
			reg:   "",
			img:   "centos:latest",
		},
		{
			name:  "empty string",
			input: "",
			reg:   "",
			img:   "",
		},
		{
			name:  "registry by ip address",
			input: "10.1.1.1:8080/image:tag",
			reg:   "10.1.1.1:8080",
			img:   "image:tag",
		},
		{
			name:  "no explicit registry with tag",
			input: "centos:latest",
			reg:   "",
			img:   "centos:latest",
		},
		{
			name:  "no explicit registry without tag",
			input: "centos",
			reg:   "",
			img:   "centos",
		},
		{
			name:  "no explicit registry with repo and image",
			input: "repository/centos",
			reg:   "",
			img:   "repository/centos",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			imp := &Tag{}
			reg, img := imp.splitRegistryDomain(tt.input)
			if reg != tt.reg {
				t.Errorf("expecting registry %q, received %q", tt.reg, reg)
			}
			if img != tt.img {
				t.Errorf("expecting image %q, received %q", tt.img, img)
			}
		})
	}
}

func TestImportPath(t *testing.T) {
	for _, tt := range []struct {
		name   string
		unqreg []string
		tag    *imagtagv1.Tag
		err    string
	}{
		{
			name: "empty tag",
			tag:  &imagtagv1.Tag{},
			err:  "empty tag reference",
		},
		{
			name: "no unqualified registry registered",
			err:  "no unqualified registries found",
			tag: &imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					From: "centos:latest",
				},
			},
		},
		{
			name:   "happy path using unqualified registry",
			unqreg: []string{"docker.io"},
			tag: &imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					From: "centos:latest",
				},
			},
		},
		{
			name: "happy path with full image reference",
			tag: &imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					From: "docker.io/centos:latest",
				},
			},
		},
		{
			name: "invalid image reference format",
			err:  "invalid reference format",
			tag: &imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					From: "docker.io/!<S87sdf<<>>",
				},
			},
		},
		{
			name:   "non existent tag",
			err:    "manifest unknown",
			unqreg: []string{"docker.io"},
			tag: &imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					From: "centos:idonotexisthopefully",
				},
			},
		},
		{
			name: "non existent registry by name",
			err:  "error pinging docker registry",
			tag: &imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					From: "i.do.not.exist.com/centos:latest",
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			corcli := corfake.NewSimpleClientset()
			corinf := coreinf.NewSharedInformerFactory(corcli, time.Minute)

			imp := &Tag{
				syssvc: NewSysContext(corinf),
			}
			imp.syssvc.unqualifiedRegistries = tt.unqreg
			_, err := imp.ImportTag(context.Background(), tt.tag)
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error %s", err)
				} else if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("unexpected error content %s", err)
				}
			} else if len(tt.err) > 0 {
				t.Errorf("expecting error %s, nil received", tt.err)
			}
		})
	}
}

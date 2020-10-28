package services

import (
	"context"
	"strings"
	"testing"
	"time"

	imagtagv1 "github.com/ricardomaraschini/it/imagetags/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	"github.com/ricardomaraschini/it/imagetags/generated/clientset/versioned/fake"
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
						imagtagv1.HashReference{
							Generation: 9,
						},
						imagtagv1.HashReference{
							Generation: 8,
						},
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
						imagtagv1.HashReference{
							Generation: 5,
						},
						imagtagv1.HashReference{
							Generation: 4,
						},
						imagtagv1.HashReference{
							Generation: 3,
						},
						imagtagv1.HashReference{
							Generation: 2,
						},
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
			err:  "not found",
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
							imagtagv1.HashReference{
								Generation: 63,
							},
							imagtagv1.HashReference{
								Generation: 62,
							},
							imagtagv1.HashReference{
								Generation: 61,
							},
							imagtagv1.HashReference{
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
			err:    "not found",
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
							imagtagv1.HashReference{
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

			fakecli := fake.NewSimpleClientset(tt.objects...)
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

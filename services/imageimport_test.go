// Copyright 2020 The Tagger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	coreinf "k8s.io/client-go/informers"
	corfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	imgv1b1 "github.com/ricardomaraschini/tagger/infra/images/v1beta1"
	imgfake "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/clientset/versioned/fake"
	imginf "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/informers/externalversions"
)

func TestImageImportSync(t *testing.T) {
	for _, tt := range []struct {
		name       string
		timp       *imgv1b1.ImageImport
		err        string
		corObjects []runtime.Object
		imgObjects []runtime.Object
		succeed    bool
	}{
		{
			name:    "already imported",
			succeed: true,
			timp: &imgv1b1.ImageImport{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "empty-img",
				},
				Spec: imgv1b1.ImageImportSpec{
					TargetImage: "empty-img",
				},
				Status: imgv1b1.ImageImportStatus{
					HashReference: &imgv1b1.HashReference{},
				},
			},
			imgObjects: []runtime.Object{
				&imgv1b1.ImageImport{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "empty-img",
					},
					Spec: imgv1b1.ImageImportSpec{
						TargetImage: "empty-img",
					},
					Status: imgv1b1.ImageImportStatus{
						HashReference: &imgv1b1.HashReference{},
					},
				},
			},
		},
		{
			name:    "max attempts",
			succeed: false,
			timp: &imgv1b1.ImageImport{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "empty-img",
				},
				Spec: imgv1b1.ImageImportSpec{
					TargetImage: "empty-img",
				},
				Status: imgv1b1.ImageImportStatus{
					ImportAttempts: []imgv1b1.ImportAttempt{
						{}, {}, {}, {}, {}, {}, {}, {}, {}, {},
					},
				},
			},
			imgObjects: []runtime.Object{
				&imgv1b1.ImageImport{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "empty-img",
					},
					Spec: imgv1b1.ImageImportSpec{
						TargetImage: "empty-img",
					},
					Status: imgv1b1.ImageImportStatus{
						ImportAttempts: []imgv1b1.ImportAttempt{
							{}, {}, {}, {}, {}, {}, {}, {}, {}, {},
						},
					},
				},
			},
		},
		{
			name:    "empty target img",
			err:     "empty spec.targetImage",
			succeed: false,
			timp: &imgv1b1.ImageImport{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "empty-img",
				},
			},
		},
		{
			name:    "first import (happy path)",
			succeed: true,
			timp: &imgv1b1.ImageImport{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "new-img",
				},
				Spec: imgv1b1.ImageImportSpec{
					TargetImage: "new-img",
					From:        "centos:latest",
				},
			},
			imgObjects: []runtime.Object{
				&imgv1b1.ImageImport{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "new-img",
					},
					Spec: imgv1b1.ImageImportSpec{
						TargetImage: "new-img",
						From:        "centos:latest",
					},
				},
				&imgv1b1.Image{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "new-img",
					},
					Spec: imgv1b1.ImageSpec{
						From: "centos:latest",
					},
				},
			},
		},
		{
			name:    "import using target img",
			succeed: true,
			timp: &imgv1b1.ImageImport{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "new-img",
				},
				Spec: imgv1b1.ImageImportSpec{
					TargetImage: "new-img",
				},
			},
			imgObjects: []runtime.Object{
				&imgv1b1.ImageImport{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "new-img",
					},
					Spec: imgv1b1.ImageImportSpec{
						TargetImage: "new-img",
					},
				},
				&imgv1b1.Image{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "new-img",
					},
					Spec: imgv1b1.ImageSpec{
						From: "centos:latest",
					},
				},
			},
		},
		{
			name:    "invalid image",
			succeed: false,
			err:     "pinging container registry does.not.exist",
			timp: &imgv1b1.ImageImport{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "new-img",
				},
				Spec: imgv1b1.ImageImportSpec{
					TargetImage: "new-img",
					From:        "does.not.exist/test/test123:latest",
				},
			},
			imgObjects: []runtime.Object{
				&imgv1b1.ImageImport{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "new-img",
					},
					Spec: imgv1b1.ImageImportSpec{
						TargetImage: "new-img",
						From:        "does.not.exist/test/test123:latest",
					},
				},
				&imgv1b1.Image{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "new-img",
					},
					Spec: imgv1b1.ImageSpec{
						From: "centos:latest",
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

			imgcli := imgfake.NewSimpleClientset(tt.imgObjects...)
			imginf := imginf.NewSharedInformerFactory(imgcli, time.Minute)

			svc := NewImageImport(corinf, imgcli, imginf)

			corinf.Start(ctx.Done())
			imginf.Start(ctx.Done())
			if !cache.WaitForCacheSync(
				ctx.Done(),
				corinf.Core().V1().ConfigMaps().Informer().HasSynced,
				corinf.Core().V1().Secrets().Informer().HasSynced,
				imginf.Tagger().V1beta1().ImageImports().Informer().HasSynced,
			) {
				t.Fatal("errors waiting for caches to sync")
			}

			err := svc.Sync(ctx, tt.timp)
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error: %s", err)
				} else if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("expecting %q, %q received instead", tt.err, err)
				}
				return
			}

			if len(tt.err) > 0 {
				t.Errorf("expecting error %q, nil received instead", tt.err)
			}

			outimp, err := imgcli.TaggerV1beta1().ImageImports("default").Get(
				ctx, tt.timp.Name, metav1.GetOptions{},
			)
			if err != nil {
				log.Fatalf("unable to find image import: %s", err)
			}

			succeed := outimp.Status.HashReference != nil
			if succeed != tt.succeed {
				fmt.Println(tt.timp.Status)
				t.Errorf("wrong succeed(%v), received %v", tt.succeed, succeed)
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
			input: "10.1.1.1:8080/image:img",
			reg:   "10.1.1.1:8080",
			img:   "image:img",
		},
		{
			name:  "no explicit registry with img",
			input: "centos:latest",
			reg:   "",
			img:   "centos:latest",
		},
		{
			name:  "no explicit registry without img",
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
			imp := &ImageImport{}
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
		timp   *imgv1b1.ImageImport
		err    string
	}{
		{
			name: "no unqualified registry registered",
			err:  "no unqualified registries found",
			timp: &imgv1b1.ImageImport{
				Spec: imgv1b1.ImageImportSpec{
					From: "centos:latest",
				},
			},
		},
		{
			name:   "happy path using unqualified registry",
			unqreg: []string{"docker.io"},
			timp: &imgv1b1.ImageImport{
				Spec: imgv1b1.ImageImportSpec{
					From: "centos:latest",
				},
			},
		},
		{
			name: "happy path with full image reference",
			timp: &imgv1b1.ImageImport{
				Spec: imgv1b1.ImageImportSpec{
					From: "docker.io/centos:latest",
				},
			},
		},
		{
			name: "invalid image reference format",
			err:  "invalid reference format",
			timp: &imgv1b1.ImageImport{
				Spec: imgv1b1.ImageImportSpec{
					From: "docker.io/!<S87sdf<<>>",
				},
			},
		},
		{
			name:   "non existent img",
			err:    "manifest unknown",
			unqreg: []string{"docker.io"},
			timp: &imgv1b1.ImageImport{
				Spec: imgv1b1.ImageImportSpec{
					From: "centos:idonotexisthopefully",
				},
			},
		},
		{
			name: "non existent registry by name",
			err:  "unable to create image closer: pinging container registry",
			timp: &imgv1b1.ImageImport{
				Spec: imgv1b1.ImageImportSpec{
					From: "i.do.not.exist.com/centos:latest",
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			corcli := corfake.NewSimpleClientset()
			corinf := coreinf.NewSharedInformerFactory(corcli, time.Minute)

			imp := &ImageImport{
				syssvc: NewSysContext(corinf),
			}
			imp.syssvc.unqualifiedRegistries = tt.unqreg

			_, err := imp.Import(context.Background(), tt.timp)
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

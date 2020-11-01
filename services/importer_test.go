package services

import (
	"context"
	"strings"
	"testing"
	"time"

	coreinf "k8s.io/client-go/informers"
	corfake "k8s.io/client-go/kubernetes/fake"

	imgtagv1 "github.com/ricardomaraschini/it/imagetags/v1"
)

func TestSplitRegistryDomain(t *testing.T) {
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
			imp := NewImporter(nil)
			reg, img := imp.SplitRegistryDomain(tt.input)
			if reg != tt.reg {
				t.Errorf("expecting reg %q, received %q", tt.reg, reg)
			}
			if img != tt.img {
				t.Errorf("expecting img %q, received %q", tt.img, img)
			}
		})
	}
}

func TestImportPath(t *testing.T) {
	for _, tt := range []struct {
		name   string
		sysctx *SysContext
		unqreg []string
		tag    *imgtagv1.Tag
		err    string
	}{
		{
			name: "empty tag",
			tag:  &imgtagv1.Tag{},
			err:  "empty tag reference",
		},
		{
			name: "no unqualified registry registered",
			err:  "no registry candidates",
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "centos:latest",
				},
			},
		},
		{
			name:   "happy path using unqualified registry",
			unqreg: []string{"docker.io"},
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "centos:latest",
				},
			},
		},
		{
			name: "happy path with full image reference",
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "docker.io/centos:latest",
				},
			},
		},
		{
			name: "invalid image reference format",
			err:  "invalid reference format",
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "docker.io/!<S87sdf<<>>",
				},
			},
		},
		{
			name:   "non existent tag",
			err:    "manifest unknown",
			unqreg: []string{"docker.io"},
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "centos:idonotexisthopefully",
				},
			},
		},
		{
			name: "non existent registry by name",
			err:  "error pinging docker registry",
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "i.do.not.exist.com/centos:latest",
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			corcli := corfake.NewSimpleClientset()
			corinf := coreinf.NewSharedInformerFactory(corcli, time.Minute)
			seclis := corinf.Core().V1().Secrets().Lister()

			sysctx := NewSysContext(seclis)
			sysctx.unqualifiedRegistries = tt.unqreg

			imp := NewImporter(sysctx)
			_, err := imp.ImportTag(ctx, tt.tag, "default")
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error %s", err)
					return
				}
				if strings.Contains(err.Error(), tt.err) {
					return
				}
				t.Errorf("unexpected error content %s", err)
				return
			}

			if len(tt.err) > 0 {
				t.Errorf("expecting error %s, nil received", tt.err)
			}
		})
	}
}

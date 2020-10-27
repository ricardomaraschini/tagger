package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
			name:  "empty strings",
			input: "",
			reg:   "",
			img:   "",
		},
		{
			name:  "ip address as registry",
			input: "10.1.1.1:8080/image:tag",
			reg:   "10.1.1.1:8080",
			img:   "image:tag",
		},
		{
			name:  "no explicit registry",
			input: "centos:latest",
			reg:   "",
			img:   "centos:latest",
		},
		{
			name:  "no explicit registry and no tag",
			input: "centos",
			reg:   "",
			img:   "centos",
		},
		{
			name:  "no explicit registry within repository",
			input: "repository/centos",
			reg:   "",
			img:   "repository/centos",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			imp := NewImporter(&SysContextMock{})
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
	fkreg := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {},
		),
	)
	defer fkreg.Close()

	for _, tt := range []struct {
		name   string
		sysctx ContextProvider
		tag    *imgtagv1.Tag
		ns     string
		err    string
	}{
		{
			name: "invalid tag name",
			tag:  &imgtagv1.Tag{},
			ns:   "default",
			err:  "invalid empty tag reference",
		},
		{
			name: "no unqualified registries",
			ns:   "default",
			err:  "no registry candidates",
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "centos:latest",
				},
			},
		},
		{
			name: "happy path using unqualified registry",
			ns:   "default",
			sysctx: &SysContextMock{
				unqualifiedRegistries: []string{"docker.io"},
			},
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "centos:latest",
				},
			},
		},
		{
			name: "happy path not using unqualified registry",
			ns:   "default",
			sysctx: &SysContextMock{
				unqualifiedRegistries: []string{"docker.io"},
			},
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "docker.io/centos:latest",
				},
			},
		},
		{
			name: "invalid image reference format",
			ns:   "default",
			err:  "invalid reference format",
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "docker.io/!<S87sdf<<>>",
				},
			},
		},
		{
			name: "non existent tag",
			ns:   "default",
			err:  "manifest unknown",
			sysctx: &SysContextMock{
				unqualifiedRegistries: []string{"docker.io"},
			},
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "centos:idonotexisthopefully",
				},
			},
		},
		{
			name: "non existent registry",
			ns:   "default",
			err:  "error pinging docker registry",
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "10.1.1.1:8888/centos:latest",
				},
			},
		},
		{
			name: "non existent registry by name",
			ns:   "default",
			err:  "error pinging docker registry",
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "i.do.not.exist.com/centos:latest",
				},
			},
		},
		{
			name: "non existent image",
			ns:   "default",
			err:  "requested access to the resource is denied",
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: "docker.io/idonotexist:latest",
				},
			},
		},
		{
			name: "http registry without insecure flag",
			ns:   "default",
			err:  "server gave HTTP response",
			tag: &imgtagv1.Tag{
				Spec: imgtagv1.TagSpec{
					From: fmt.Sprintf("%s/image:tag", fkreg.Listener.Addr()),
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var sysctx ContextProvider

			sysctx = &SysContextMock{}
			if tt.sysctx != nil {
				sysctx = tt.sysctx
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			imp := NewImporter(sysctx)
			_, err := imp.ImportTag(ctx, tt.tag, tt.ns)
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

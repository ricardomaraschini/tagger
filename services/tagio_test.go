package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinf "k8s.io/client-go/informers"
	corfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/containers/image/v5/types"
	"github.com/ricardomaraschini/tagger/infra/fs"
	imagtagv1 "github.com/ricardomaraschini/tagger/infra/tags/v1"
	tagfake "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/clientset/versioned/fake"
	taginf "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/informers/externalversions"
)

func TestPull(t *testing.T) {
	for _, tt := range []struct {
		name    string
		tagname string
		tagns   string
		err     string
		regaddr string
		tagobj  *imagtagv1.Tag
	}{
		{
			name:    "tag not found",
			tagname: "a-tag",
			tagns:   "a-namespace",
			err:     "not found",
		},
		{
			name:    "cache registry not configured",
			tagname: "another-tag",
			tagns:   "another-namespace",
			err:     "unable to find cache registry",
			tagobj: &imagtagv1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-tag",
					Namespace: "another-namespace",
				},
			},
		},
		{
			name:    "happy path",
			tagname: "another-tag",
			tagns:   "another-namespace",
			regaddr: "quay.io",
			tagobj: &imagtagv1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-tag",
					Namespace: "another-namespace",
				},
				Spec: imagtagv1.TagSpec{
					From:       "quay.io/rmarasch/tagger-e2e-tests:scratch-v2",
					Generation: 1,
				},
				Status: imagtagv1.TagStatus{
					Generation:        0,
					LastImportAttempt: imagtagv1.ImportAttempt{},
					References: []imagtagv1.HashReference{
						{
							Generation:     1,
							From:           "quay.io/rmarasch/tagger-e2e-tests:scratch-v2",
							ImageReference: "quay.io/rmarasch/tagger-e2e-tests@sha256:b6ce06db25ae667c75151edbde443b803b23d9a5e060dcbab1fa968b4115286d",
						},
						{
							Generation:     0,
							From:           "quay.io/rmarasch/tagger-e2e-tests:scratch-v1",
							ImageReference: "quay.io/rmarasch/tagger-e2e-tests@sha256:b2ec0d3c72aa7bb987bf20dd27b05b3795e7379a852b4678b06ab822a797013b",
						},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			if len(tt.regaddr) > 0 {
				os.Setenv("CACHE_REGISTRY_ADDRESS", tt.regaddr)
				defer os.Unsetenv("CACHE_REGISTRY_ADDRESS")
			}

			corcli := corfake.NewSimpleClientset()
			corinf := coreinf.NewSharedInformerFactory(corcli, time.Minute)

			tagcli := tagfake.NewSimpleClientset()
			if tt.tagobj != nil {
				tagcli = tagfake.NewSimpleClientset(tt.tagobj)
			}
			taginf := taginf.NewSharedInformerFactory(tagcli, time.Minute)

			tagio := NewTagIO(corinf, tagcli, taginf)
			fs := fs.New("")
			tagio.fstsvc = fs
			tagio.impsvc.fs = fs

			taginf.Start(ctx.Done())
			if !cache.WaitForCacheSync(
				ctx.Done(),
				taginf.Images().V1().Tags().Informer().HasSynced,
			) {
				t.Fatal("errors waiting for caches to sync")
			}

			ch := make(chan types.ProgressProperties)
			go func() {
				for range ch {
				}
			}()
			tarfp, clean, err := tagio.Pull(ctx, tt.tagns, tt.tagname, ch)
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error: %s", err)
				} else if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("expecting %q, %q received instead", tt.err, err)
				}
			} else if len(tt.err) > 0 {
				t.Errorf("expecting error %q, nil received instead", tt.err)
			}
			close(ch)

			// tar was not created on this test, move on.
			if tarfp == nil {
				return
			}
			defer clean()

			dst, cleanup, err := tagio.fstsvc.TempDir()
			if err != nil {
				t.Fatalf("unexpected error creating temp dir: %s", err)
			}
			defer cleanup()

			if err := tagio.fstsvc.UnarchiveFile(tarfp, dst); err != nil {
				t.Fatalf("error unarchiving file: %s", err)
			}

			tagfile := fmt.Sprintf("%s/tag.json", dst)
			fp, err := os.Open(tagfile)
			if err != nil {
				t.Errorf("tag.json not found: %s", err)
				return
			}
			fp.Close()

			for i := 0; i < len(tt.tagobj.Status.References); i++ {
				manpath := fmt.Sprintf("%s/%d-manifest.json", dst, i)
				fp, err := os.Open(manpath)
				if err != nil {
					t.Errorf("manifest %s not found: %s", manpath, err)
					return
				}
				fp.Close()

				versionpath := fmt.Sprintf("%s/%d-version", dst, i)
				fp, err = os.Open(versionpath)
				if err != nil {
					t.Errorf("version %s not found: %s", versionpath, err)
					return
				}
				fp.Close()

				blobspath := fmt.Sprintf("%s/blobs", dst)
				fp, err = os.Open(blobspath)
				if err != nil {
					t.Errorf("blobs directory not found: %s", err)
					return
				}
				fp.Close()
			}
		})
	}
}

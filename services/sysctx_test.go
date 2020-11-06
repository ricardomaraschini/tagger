package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	coreinf "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/types"
)

func TestCreateSelfSignedCertificate(t *testing.T) {
	key, crt, err := NewSysContext(nil, nil).CreateSelfSignedCertificate()
	if err != nil {
		t.Errorf("unable to create self signed certificate: %s", err)
	}

	t.Log(string(key))
	t.Log(string(crt))
}

func TestUnqualifiedRegistries(t *testing.T) {
	unq := NewSysContext(nil, nil).UnqualifiedRegistries(context.Background())
	if len(unq) != 1 {
		t.Fatal("expecting only one unqualified registry")
	}
	if unq[0] != "docker.io" {
		t.Fatalf("expecting registry docker.io, received %s instead", unq[0])
	}
}

func TestAuthsFor(t *testing.T) {
	auths, _ := json.Marshal(
		dockerAuthConfig{
			Auths: map[string]types.DockerAuthConfig{
				"docker.io": types.DockerAuthConfig{
					Username: "user",
					Password: "pass",
				},
				"quay.io": types.DockerAuthConfig{
					Username: "another-user",
					Password: "another-pass",
				},
			},
		},
	)

	for _, tt := range []struct {
		name       string
		image      string
		err        string
		authsCount int
		objects    []runtime.Object
	}{
		{
			name:  "no auths",
			image: "centos:latest",
		},
		{
			name:  "secret without type present on namespace",
			image: "centos:latest",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "a secret",
					},
				},
			},
		},
		{
			name:  "secret with right type but no 'data' map entry",
			image: "centos:latest",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "secret",
					},
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{},
				},
			},
		},
		{
			name:  "secret right type but with invalid auth configuration",
			image: "centos:latest",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "secret",
					},
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						corev1.DockerConfigJsonKey: []byte("MTIz"),
					},
				},
			},
		},
		{
			name:       "happy path with one auth present for docker.io",
			image:      "centos:latest",
			authsCount: 1,
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "secret",
					},
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						corev1.DockerConfigJsonKey: auths,
					},
				},
			},
		},
		{
			name:       "happy path with multiple auths for docker.io",
			image:      "centos:latest",
			authsCount: 2,
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "secret",
					},
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						corev1.DockerConfigJsonKey: auths,
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "another-secret",
					},
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						corev1.DockerConfigJsonKey: auths,
					},
				},
			},
		},
		{
			name:       "no auth present for specific registry",
			image:      "192.168.10.1/repo/image:latest",
			authsCount: 0,
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "secret",
					},
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						corev1.DockerConfigJsonKey: auths,
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
			seclis := informer.Core().V1().Secrets().Lister()
			cmlist := informer.Core().V1().ConfigMaps().Lister()
			informer.Start(ctx.Done())
			if !cache.WaitForCacheSync(
				ctx.Done(),
				informer.Core().V1().Secrets().Informer().HasSynced,
				informer.Core().V1().ConfigMaps().Informer().HasSynced,
			) {
				t.Fatal("errors waiting for caches to sync")
			}

			ref, _ := reference.ParseDockerRef(tt.image)
			imgref, _ := docker.NewReference(ref)
			sysctx := NewSysContext(cmlist, seclis)

			auths, err := sysctx.AuthsFor(ctx, imgref, "default")
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error %s", err)
					return
				}
				if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("invalid error %s", err.Error())
				}
			} else if len(tt.err) > 0 {
				t.Errorf("expecting error %s, nil received instead", tt.err)
			}

			if len(auths) != tt.authsCount {
				t.Errorf("expecting %d, %d received", tt.authsCount, len(auths))
			}
		})
	}
}

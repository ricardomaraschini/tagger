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
	"encoding/json"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	corelister "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/types"
	"github.com/hashicorp/go-multierror"
	"github.com/ricardomaraschini/tagger/infra/imagestore"
	"gopkg.in/yaml.v2"
)

// We use dockerAuthConfig to unmarshal a default docker configuration present on secrets of
// type SecretTypeDockerConfigJson. XXX doesn't containers/image export a similar structure?
// Or maybe even a function to parse a docker configuration file?
type dockerAuthConfig struct {
	Auths map[string]types.DockerAuthConfig
}

// MirrorRegistryConfig holds the needed data that allows tagger to contact the mirror registry.
type MirrorRegistryConfig struct {
	Address    string
	Username   string
	Password   string
	Repository string
	Token      string
	Insecure   bool
}

// LocalRegistryHostingV1 describes a local registry that developer tools can connect to. A local
// registry allows clients to load images into the local cluster by pushing to this registry.
// This is a verbatim copy of what is in the enhancement proposal at
// https://github.com/kubernetes/enhancements repo
// keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
type LocalRegistryHostingV1 struct {
	// Host documents the host (hostname and port) of the registry, as seen from outside the
	// cluster. This is the registry host that tools outside the cluster should push images
	// to.
	Host string `yaml:"host,omitempty"`

	// HostFromClusterNetwork documents the host (hostname and port) of the registry, as seen
	// from networking inside the container pods. This is the registry host that tools running
	// on pods inside the cluster should push images to. If not set, then tools inside the
	// cluster should assume the local registry is not available to them.
	HostFromClusterNetwork string `yaml:"hostFromClusterNetwork,omitempty"`

	// HostFromContainerRuntime documents the host (hostname and port) of the registry, as
	// seen from the cluster's container runtime. When tools apply Kubernetes objects to the
	// cluster, this host should be used for image name fields. If not set, users of this
	// field should use the value of Host instead. Note that it doesn't make sense
	// semantically to define this field, but not define Host or HostFromClusterNetwork. That
	// would imply a way to pull images without a way to push images.
	HostFromContainerRuntime string `yaml:"hostFromContainerRuntime,omitempty"`

	// Help contains a URL pointing to documentation for users on how to set up and configure
	// a local registry. Tools can use this to nudge users to enable the registry.
	// When possible, the writer should use as permanent a URL as possible to prevent drift
	// (e.g., a version control SHA). When image pushes to a registry host specified in one of
	// the other fields fail, the tool should display this help URL to the user. The help URL
	// should contain instructions on how to diagnose broken or misconfigured registries.
	Help string `yaml:"help,omitempty"`
}

// SysContext groups tasks related to system context/configuration, deal with things such as
// configured docker authentications or unqualified registries configs.
type SysContext struct {
	sclister              corelister.SecretLister
	cmlister              corelister.ConfigMapLister
	unqualifiedRegistries []string
}

// NewSysContext returns a new SysContext helper.
func NewSysContext(corinf informers.SharedInformerFactory) *SysContext {
	var sclister corelister.SecretLister
	var cmlister corelister.ConfigMapLister
	if corinf != nil {
		sclister = corinf.Core().V1().Secrets().Lister()
		cmlister = corinf.Core().V1().ConfigMaps().Lister()
	}

	return &SysContext{
		sclister:              sclister,
		cmlister:              cmlister,
		unqualifiedRegistries: []string{"docker.io"},
	}
}

// UnqualifiedRegistries returns the list of unqualified registries configured on the system.
// XXX this is a place holder as we most likely gonna need to read this from a configuration
// somewhere.
func (s *SysContext) UnqualifiedRegistries(ctx context.Context) ([]string, error) {
	return s.unqualifiedRegistries, nil
}

// ParseMirrorRegistryConfig reads configmap local-registry-hosting from kube-public namespace,
// parses its content and returns the local registry configuration.
func (s *SysContext) ParseMirrorRegistryConfig() (*LocalRegistryHostingV1, error) {
	cm, err := s.cmlister.ConfigMaps("kube-public").Get("local-registry-hosting")
	if err != nil {
		return nil, fmt.Errorf("error getting registry configmap: %w", err)
	}

	dt, ok := cm.Data["localRegistryHosting.v1"]
	if !ok {
		return nil, fmt.Errorf("configmap index localRegistryHosting.v1 not found")
	}

	cfg := &LocalRegistryHostingV1{}
	if err := yaml.Unmarshal([]byte(dt), cfg); err != nil {
		return nil, fmt.Errorf("unable to unmarshal registry config: %w", err)
	}
	return cfg, nil
}

// MirrorConfig returns the mirror configuration as read from Tagger namespace or from the
// kube-public namespace as per KEP.
func (s *SysContext) MirrorConfig() (MirrorRegistryConfig, error) {
	var errors *multierror.Error
	taggercfg, err := s.ParseTaggerMirrorRegistryConfig()
	if err == nil {
		return taggercfg, nil
	}
	multierror.Append(errors, err)

	kubecfg, err := s.ParseMirrorRegistryConfig()
	if err != nil {
		multierror.Append(errors, err)
		return MirrorRegistryConfig{}, fmt.Errorf("unable to config mirror: %w", errors)
	}

	return MirrorRegistryConfig{
		Address: kubecfg.HostFromContainerRuntime,
	}, nil
}

// ParseTaggerMirrorRegistryConfig parses a secret called "mirror-registry-config" in the pod
// namespace. This secret holds information on how to connect to the mirror registry.
func (s *SysContext) ParseTaggerMirrorRegistryConfig() (MirrorRegistryConfig, error) {
	var zero MirrorRegistryConfig

	namespace := os.Getenv("POD_NAMESPACE")
	if len(namespace) == 0 {
		return zero, fmt.Errorf("unbound POD_NAMESPACE variable")
	}

	sct, err := s.sclister.Secrets(namespace).Get("mirror-registry-config")
	if err != nil {
		return zero, fmt.Errorf("unable to read registry config: %w", err)
	}
	if len(sct.Data) == 0 {
		return zero, fmt.Errorf("registry config is empty")
	}

	return MirrorRegistryConfig{
		Address:    string(sct.Data["address"]),
		Username:   string(sct.Data["username"]),
		Password:   string(sct.Data["password"]),
		Repository: string(sct.Data["repository"]),
		Token:      string(sct.Data["token"]),
		Insecure:   string(sct.Data["insecure"]) == "true",
	}, nil
}

// MirrorRegistryAddresses returns the configured registry address used for mirroring images.
// This is implemented to comply with KEP at https://github.com/kubernetes/enhancements/ repo,
// see keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry. There are two
// ways of providing the mirror registry information, the first one is to populate a secret
// in the current namespace, the other one is by complying with the KEP. We give preference
// for the secret in the current namespace.
func (s *SysContext) MirrorRegistryAddresses() (string, string, error) {
	var errors *multierror.Error
	cfg, err := s.ParseTaggerMirrorRegistryConfig()
	if err == nil {
		return cfg.Address, cfg.Address, nil
	}
	multierror.Append(errors, err)

	// moves to check through the KEP implementation.
	kepcfg, err := s.ParseMirrorRegistryConfig()
	if err != nil {
		multierror.Append(errors, err)
		return "", "", fmt.Errorf("mirror registry address unknown: %w", errors)
	}
	return kepcfg.HostFromClusterNetwork, kepcfg.HostFromContainerRuntime, nil
}

// MirrorRegistryContext returns the context to be used when talking to the the registry used
// for mirroring images.
func (s *SysContext) MirrorRegistryContext(ctx context.Context) *types.SystemContext {
	cfg, err := s.ParseTaggerMirrorRegistryConfig()
	if err != nil {
		klog.Infof("unable to read tagger mirror registry config: %s", err)
	}

	insecure := types.OptionalBoolFalse
	if cfg.Insecure {
		insecure = types.OptionalBoolTrue
	}

	return &types.SystemContext{
		DockerInsecureSkipTLSVerify: insecure,
		DockerAuthConfig: &types.DockerAuthConfig{
			Username:      cfg.Username,
			Password:      cfg.Password,
			IdentityToken: cfg.Token,
		},
	}
}

// SystemContextsFor builds a series of types.SystemContexts, all of them using one of the auth
// credentials present in the namespace. The last entry is always a nil SystemContext, this last
// entry means "no auth". Insecure indicate if the returned SystemContexts tolerate invalid TLS
// certificates.
func (s *SysContext) SystemContextsFor(
	ctx context.Context,
	imgref types.ImageReference,
	namespace string,
	insecure bool,
) ([]*types.SystemContext, error) {
	// if imgref points to an image hosted in our mirror registry we return a SystemContext
	// using default user and pass (the ones user has configured tagger with). XXX i am not
	// sure yet this is a good idea permission wide.
	domain := reference.Domain(imgref.DockerReference())
	regaddr, _, err := s.MirrorRegistryAddresses()
	if err != nil {
		klog.Infof("no mirror registry configured, moving on")
	} else if regaddr == domain {
		mirrorctx := s.MirrorRegistryContext(ctx)
		return []*types.SystemContext{mirrorctx}, nil
	}

	auths, err := s.authsFor(ctx, imgref, namespace)
	if err != nil {
		return nil, fmt.Errorf("error reading auths: %w", err)
	}

	optinsecure := types.OptionalBoolFalse
	if insecure {
		optinsecure = types.OptionalBoolTrue
	}

	ctxs := make([]*types.SystemContext, len(auths))
	for i, auth := range auths {
		ctxs[i] = &types.SystemContext{
			DockerInsecureSkipTLSVerify: optinsecure,
			DockerAuthConfig:            auth,
		}
	}

	// here we append a SystemContext without authentications set, we want to allow imports
	// without using authentication. This entry will be nil if we want to use the system
	// defaults.
	var noauth *types.SystemContext
	if insecure {
		noauth = &types.SystemContext{
			DockerInsecureSkipTLSVerify: optinsecure,
		}
	}

	ctxs = append(ctxs, noauth)
	return ctxs, nil
}

// authsFor return configured authentications for the registry hosting the image reference.
// Namespace is the namespace from where read docker authentications.
func (s *SysContext) authsFor(
	ctx context.Context, imgref types.ImageReference, namespace string,
) ([]*types.DockerAuthConfig, error) {
	secrets, err := s.sclister.Secrets(namespace).List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("fail to list secrets: %w", err)
	}

	domain := reference.Domain(imgref.DockerReference())
	if domain == "" {
		return nil, nil
	}

	var dockerAuths []*types.DockerAuthConfig
	for _, sec := range secrets {
		if sec.Type != corev1.SecretTypeDockerConfigJson {
			continue
		}

		secdata, ok := sec.Data[corev1.DockerConfigJsonKey]
		if !ok {
			continue
		}

		var cfg dockerAuthConfig
		if err := json.Unmarshal(secdata, &cfg); err != nil {
			klog.Infof("ignoring secret %s/%s: %s", sec.Namespace, sec.Name, err)
			continue
		}

		sec, ok := cfg.Auths[domain]
		if !ok {
			continue
		}

		dockerAuths = append(dockerAuths, &sec)
	}
	return dockerAuths, nil
}

// DefaultPolicyContext returns the default policy context. XXX this should be reviewed.
func (s *SysContext) DefaultPolicyContext() (*signature.PolicyContext, error) {
	pol := &signature.Policy{
		Default: signature.PolicyRequirements{
			signature.NewPRInsecureAcceptAnything(),
		},
	}
	return signature.NewPolicyContext(pol)
}

// GetRegistryStore creates an instance of an Registry store entity configured to use our mirror
// registry as underlying storage.
func (s *SysContext) GetRegistryStore(ctx context.Context) (*imagestore.Registry, error) {
	defpol, err := s.DefaultPolicyContext()
	if err != nil {
		return nil, fmt.Errorf("error reading default policy: %w", err)
	}

	mcfg, err := s.MirrorConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to acccess mirror: %w", err)
	}

	sysctx := s.MirrorRegistryContext(ctx)
	return imagestore.NewRegistry(mcfg.Address, mcfg.Repository, sysctx, defpol), nil
}

// RegistriesToSearch returns a list of registries to be used when looking for an image. It is
// either the provided domain or a list of unqualified domains configured globally and returned
// by UnqualifiedRegistries(). This function is used when trying to understand what an user means
// when she/he simply asks to import an image called "centos:latest" for instance, in what
// registries do we need to look for this image? This is the place where we can implement a mirror
// search.
func (s *SysContext) RegistriesToSearch(ctx context.Context, domain string) ([]string, error) {
	if domain != "" {
		return []string{domain}, nil
	}
	registries, err := s.UnqualifiedRegistries(ctx)
	if err != nil {
		return nil, err
	}

	if len(registries) == 0 {
		return nil, fmt.Errorf("no unqualified registries found")
	}
	return registries, nil
}

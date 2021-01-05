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

	"gopkg.in/yaml.v2"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/types"
)

// We use dockerAuthConfig to unmarshal a default docker configuration present on
// secrets of type SecretTypeDockerConfigJson. XXX doesn't containers/image export
// a similar structure? Of maybe even a function to parse a docker configuration
// file?
type dockerAuthConfig struct {
	Auths map[string]types.DockerAuthConfig
}

// LocalRegistryHostingV1 describes a local registry that developer tools can
// connect to. A local registry allows clients to load images into the local
// cluster by pushing to this registry. This is a verbatim copy of what is
// on the enhancement proposal in https://github.com/kubernetes/enhancements/
// repo: keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
type LocalRegistryHostingV1 struct {
	// Host documents the host (hostname and port) of the registry, as seen from
	// outside the cluster.
	//
	// This is the registry host that tools outside the cluster should push images
	// to.
	Host string `yaml:"host,omitempty"`

	// HostFromClusterNetwork documents the host (hostname and port) of the
	// registry, as seen from networking inside the container pods.
	//
	// This is the registry host that tools running on pods inside the cluster
	// should push images to. If not set, then tools inside the cluster should
	// assume the local registry is not available to them.
	HostFromClusterNetwork string `yaml:"hostFromClusterNetwork,omitempty"`

	// HostFromContainerRuntime documents the host (hostname and port) of the
	// registry, as seen from the cluster's container runtime.
	//
	// When tools apply Kubernetes objects to the cluster, this host should be
	// used for image name fields. If not set, users of this field should use the
	// value of Host instead.
	//
	// Note that it doesn't make sense semantically to define this field, but not
	// define Host or HostFromClusterNetwork. That would imply a way to pull
	// images without a way to push images.
	HostFromContainerRuntime string `yaml:"hostFromContainerRuntime,omitempty"`

	// Help contains a URL pointing to documentation for users on how to set
	// up and configure a local registry.
	//
	// Tools can use this to nudge users to enable the registry. When possible,
	// the writer should use as permanent a URL as possible to prevent drift
	// (e.g., a version control SHA).
	//
	// When image pushes to a registry host specified in one of the other fields
	// fail, the tool should display this help URL to the user. The help URL
	// should contain instructions on how to diagnose broken or misconfigured
	// registries.
	Help string `yaml:"help,omitempty"`
}

// SysContext groups tasks related to system context/configuration, deal
// with things such as configured docker authentications or unqualified
// registries configs.
type SysContext struct {
	sclister              corelister.SecretLister
	cmlister              corelister.ConfigMapLister
	unqualifiedRegistries []string
}

// NewSysContext returns a new SysContext helper.
func NewSysContext(corinf informers.SharedInformerFactory) *SysContext {
	return &SysContext{
		sclister:              corinf.Core().V1().Secrets().Lister(),
		cmlister:              corinf.Core().V1().ConfigMaps().Lister(),
		unqualifiedRegistries: []string{"docker.io"},
	}
}

// UnqualifiedRegistries returns the list of unqualified registries
// configured on the system. XXX here we should return the cluster
// wide configuration for unqualified registries.
func (s *SysContext) UnqualifiedRegistries(ctx context.Context) []string {
	return s.unqualifiedRegistries
}

// parseCacheRegistryConfig reads configmap local-registry-hosting from kube-public
// namespace, parses its content and returns the local registry configuration.
func (s *SysContext) parseCacheRegistryConfig() (*LocalRegistryHostingV1, error) {
	cm, err := s.cmlister.ConfigMaps("kube-public").Get("local-registry-hosting")
	if err != nil {
		return nil, err
	}

	dt, ok := cm.Data["localRegistryHosting.v1"]
	if !ok {
		return nil, fmt.Errorf("no v1 local registry config found")
	}

	cfg := &LocalRegistryHostingV1{}
	if err := yaml.Unmarshal([]byte(dt), cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// CacheRegistryAddresses returns the configured registry address used
// for caching images during tags. This is implemented to comply with
// KEP at https://github.com/kubernetes/enhancements/ repository, see
// keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
// We evaluate if CACHE_REGISTRY_ADDRESS environment variable is set
// before moving on to the implementation following the KEP. This returns
// one address for connections starting from within the cluster and another
// for connections started from the cluster container runtime.
func (s *SysContext) CacheRegistryAddresses() (string, string, error) {
	if addr := os.Getenv("CACHE_REGISTRY_ADDRESS"); len(addr) > 0 {
		return addr, addr, nil
	}

	cfg, err := s.parseCacheRegistryConfig()
	if err != nil {
		return "", "", err
	}
	return cfg.HostFromClusterNetwork, cfg.HostFromContainerRuntime, nil
}

// CacheRegistryContext returns the context to be used when talking to
// the the registry used for caching tags.
func (s *SysContext) CacheRegistryContext(ctx context.Context) *types.SystemContext {
	insecure := types.OptionalBoolFalse
	if os.Getenv("CACHE_REGISTRY_INSECURE") != "" {
		insecure = types.OptionalBoolTrue
	}
	return &types.SystemContext{
		DockerInsecureSkipTLSVerify: insecure,
		DockerAuthConfig: &types.DockerAuthConfig{
			Username:      os.Getenv("CACHE_REGISTRY_USERNAME"),
			Password:      os.Getenv("CACHE_REGISTRY_PASSWORD"),
			IdentityToken: os.Getenv("CACHE_REGISTRY_TOKEN"),
		},
	}
}

// AuthsFor return configured authentications for the registry hosting
// the image reference. Namespace is the namespace from where read docker
// authentications.
func (s *SysContext) AuthsFor(
	ctx context.Context, imgref types.ImageReference, namespace string,
) ([]*types.DockerAuthConfig, error) {
	// XXX get secrets by type?
	secrets, err := s.sclister.Secrets(namespace).List(labels.Everything())
	if err != nil {
		return nil, err
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

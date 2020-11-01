package services

import (
	"context"
	"encoding/json"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corelister "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/types"
)

// SysContext groups tasks related to system context/configuration, deal
// with things such as configured docker authentications or unqualified
// registries configs.
type SysContext struct {
	sclister              corelister.SecretLister
	unqualifiedRegistries []string
}

// NewSysContext returns a new SysContext helper.
func NewSysContext(lister corelister.SecretLister) *SysContext {
	return &SysContext{
		sclister:              lister,
		unqualifiedRegistries: []string{"docker.io"},
	}
}

// UnqualifiedRegistries returns the list of unqualified registries
// configured on the system. XXX here we should return the cluster
// wide configuration for unqualified registries.
func (s *SysContext) UnqualifiedRegistries(ctx context.Context) []string {
	return s.unqualifiedRegistries
}

// CacheRegistryAddr returns the configured registry address used for
// caching images during tags.
func (s *SysContext) CacheRegistryAddr() string {
	return os.Getenv("CACHE_REGISTRY_ADDRESS")
}

// CacheRegistryContext returns the context to be used when talking to
// the the registry used for caching tags.
func (s *SysContext) CacheRegistryContext(ctx context.Context) *types.SystemContext {
	return &types.SystemContext{
		DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
		DockerAuthConfig: &types.DockerAuthConfig{
			Username: os.Getenv("CACHE_REGISTRY_USERNAME"),
			Password: os.Getenv("CACHE_REGISTRY_PASSWORD"),
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

// We use dockerAuthConfig to unmarshal a default docker configuration
// present on secrets of type SecretTypeDockerConfigJson. XXX doesn't
// containers/image export a similar structure? Of maybe even a function
// to parse a docker configuration file?
type dockerAuthConfig struct {
	Auths map[string]types.DockerAuthConfig
}

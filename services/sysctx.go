package services

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corelister "k8s.io/client-go/listers/core/v1"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/types"
)

// SysContext groups tasks related to system context/configuration, deal
// with things such as configured docker authentications or unqualified
// registries configs.
type SysContext struct {
	sclister corelister.SecretLister
}

// NewSysContext returns a new SysContext helper.
func NewSysContext(lister corelister.SecretLister) *SysContext {
	return &SysContext{
		sclister: lister,
	}
}

// UnqualifiedRegistries returns the list of unqualified registries
// configured on the system. XXX here we should return the cluster
// wide configuration for unqualified registries, for now we hardcoded
// docker.io for testing purposes.
func (s *SysContext) UnqualifiedRegistries(ctx context.Context) []string {
	return []string{"docker.io"}
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

	dockerAuths := []*types.DockerAuthConfig{}
	domain := reference.Domain(imgref.DockerReference())
	for _, secret := range secrets {
		if secret.Type != corev1.SecretTypeDockerConfigJson {
			continue
		}

		secdata, ok := secret.Data[corev1.DockerConfigJsonKey]
		if !ok {
			continue
		}

		var cfg dockerAuthConfig
		if err := json.Unmarshal(secdata, &cfg); err != nil {
			return nil, err
		}

		sec, ok := cfg.Auths[domain]
		if !ok {
			continue
		}

		dockerAuths = append(dockerAuths, &sec)
	}

	if len(dockerAuths) == 0 || domain == "" {
		return []*types.DockerAuthConfig{nil}, nil
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

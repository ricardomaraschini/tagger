package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	"github.com/hashicorp/go-multierror"

	imagtagv1 "github.com/ricardomaraschini/it/imagetags/v1"
)

// Importer wrap srvices for tag import related operations.
type Importer struct {
	syssvc *SysContext
}

// NewImporter returns a handler for tag related services.
func NewImporter(syssvc *SysContext) *Importer {
	return &Importer{
		syssvc: syssvc,
	}
}

// SplitRegistryDomain splits the domain from the repository and image.
func (i *Importer) SplitRegistryDomain(imgPath string) (string, string) {
	imageSlices := strings.SplitN(imgPath, "/", 2)
	if len(imageSlices) < 2 {
		return "", imgPath
	}

	// if domain does not contain ".", ":" and is not "localhost"
	// we don't consider it a domain at all, return empty.
	if !strings.ContainsAny(imageSlices[0], ".:") && imageSlices[0] != "localhost" {
		return "", imgPath
	}

	return imageSlices[0], imageSlices[1]
}

// ImportTag runs an import on provided Tag.
func (i *Importer) ImportTag(
	ctx context.Context, it *imagtagv1.Tag, namespace string,
) (imagtagv1.HashReference, error) {
	var zero imagtagv1.HashReference
	if it.Spec.From == "" {
		return zero, fmt.Errorf("empty tag reference")
	}

	regDomain, remainder := i.SplitRegistryDomain(it.Spec.From)

	registries := i.syssvc.UnqualifiedRegistries(ctx)
	if regDomain != "" {
		registries = []string{regDomain}
	}
	if len(registries) == 0 {
		return zero, fmt.Errorf("no registry candidates found")
	}

	var errors *multierror.Error
	for _, registry := range registries {
		imgFullPath := fmt.Sprintf("%s/%s", registry, remainder)
		namedReference, err := reference.ParseDockerRef(imgFullPath)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		imgref, err := docker.NewReference(namedReference)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		auths, err := i.syssvc.AuthsFor(ctx, imgref, namespace)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}
		// adds a no authenticated attempt to the last position so
		// if everything fails we attempt without auth at all.
		auths = append(auths, nil)

		for _, auth := range auths {
			sysctx := &types.SystemContext{
				DockerAuthConfig: auth,
			}

			// XXX move this to its own func.
			img, err := imgref.NewImage(ctx, sysctx)
			if err != nil {
				errors = multierror.Append(errors, err)
				continue
			}

			manifestBlob, _, err := img.Manifest(ctx)
			if err != nil {
				img.Close()
				errors = multierror.Append(errors, err)
				continue
			}
			defer img.Close()

			dgst, err := manifest.Digest(manifestBlob)
			if err != nil {
				return zero, fmt.Errorf("error calculating manifest digest: %w", err)
			}

			imageref := fmt.Sprintf("%s@%s", imgref.DockerReference().Name(), dgst)
			return imagtagv1.HashReference{
				Generation:     it.Spec.Generation,
				From:           it.Spec.From,
				ImportedAt:     metav1.NewTime(time.Now()),
				ImageReference: imageref,
			}, nil
		}
	}
	return zero, errors.ErrorOrNil()
}

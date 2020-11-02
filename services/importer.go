package services

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imgcopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature"
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

// ImageRefForStringRef parses provided string into a types.ImageReference.
func (i *Importer) ImageRefForStringRef(ref string) (types.ImageReference, error) {
	namedReference, err := reference.ParseDockerRef(ref)
	if err != nil {
		return nil, err
	}
	imgRef, err := docker.NewReference(namedReference)
	if err != nil {
		return nil, err
	}
	return imgRef, nil
}

// DefaultPolicyContext returns the default policy context. XXX this should
// be reviewed.
func (i *Importer) DefaultPolicyContext() (*signature.PolicyContext, error) {
	defpol, err := signature.DefaultPolicy(nil)
	if err != nil {
		return nil, err
	}
	return signature.NewPolicyContext(defpol)
}

// cacheTag copies an image from one registry to another. The first is
// the source registry, the latter is our caching registry. Returns the
// cached image reference to be used.
func (i *Importer) cacheTag(
	ctx context.Context,
	from string,
	it *imagtagv1.Tag,
	srcCtx *types.SystemContext,
) (string, error) {
	fromRef, err := i.ImageRefForStringRef(from)
	if err != nil {
		return "", err
	}

	regaddr, err := i.syssvc.CacheRegistryAddr()
	if err != nil {
		return "", err
	}

	// We cache images under registry/namespace/image-tag.
	to := fmt.Sprintf("%s/%s/%s", regaddr, it.Namespace, it.Name)
	toRef, err := i.ImageRefForStringRef(to)
	if err != nil {
		return "", err
	}

	polctx, err := i.DefaultPolicyContext()
	if err != nil {
		return "", err
	}

	manifest, err := imgcopy.Image(
		ctx, polctx, toRef, fromRef, &imgcopy.Options{
			ImageListSelection: imgcopy.CopyAllImages,
			SourceCtx:          srcCtx,
			DestinationCtx:     i.syssvc.CacheRegistryContext(ctx),
		},
	)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"%s/%s/%s@sha256:%x", regaddr, it.Namespace, it.Name, sha256.Sum256(manifest),
	), nil
}

// ImportTag runs an import on provided Tag.
func (i *Importer) ImportTag(
	ctx context.Context, it *imagtagv1.Tag,
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

		auths, err := i.syssvc.AuthsFor(ctx, imgref, it.Namespace)
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
			if it.Spec.Cache {
				imageref, err = i.cacheTag(ctx, imageref, it, sysctx)
				if err != nil {
					return zero, err
				}
			}

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

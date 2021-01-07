package services

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"

	imgcopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/types"
	"github.com/hashicorp/go-multierror"

	imagtagv1 "github.com/ricardomaraschini/tagger/imagetags/v1"
)

// Importer wrap srvices for tag import related operations.
type Importer struct {
	syssvc *SysContext
	metric *Metric
}

// NewImporter returns a handler for tag related services. I have chosen to go
// with a lazy approach here, you can pass or omit (nil) the argument, it is
// up to the caller to decide what is needed for each specific case. So far this
// is the best approach, I still plan to review this.
func NewImporter(corinf informers.SharedInformerFactory) *Importer {
	return &Importer{
		syssvc: NewSysContext(corinf),
		metric: NewMetrics(),
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
	return docker.NewReference(namedReference)
}

// DefaultPolicyContext returns the default policy context. XXX this should
// be reviewed.
func (i *Importer) DefaultPolicyContext() (*signature.PolicyContext, error) {
	pol := &signature.Policy{
		Default: signature.PolicyRequirements{
			signature.NewPRInsecureAcceptAnything(),
		},
	}
	return signature.NewPolicyContext(pol)
}

// cacheTag copies an image from one registry to another. The first is
// the source registry, the latter is our caching registry. Returns the
// cached image reference to be used.
func (i *Importer) cacheTag(
	ctx context.Context,
	it *imagtagv1.Tag,
	from string,
	srcCtx *types.SystemContext,
) (string, error) {
	fromRef, err := i.ImageRefForStringRef(from)
	if err != nil {
		return "", err
	}

	inregaddr, outregaddr, err := i.syssvc.CacheRegistryAddresses()
	if err != nil {
		return "", err
	}

	// We cache images under registry/namespace/image-tag.
	to := fmt.Sprintf("%s/%s/%s", inregaddr, it.Namespace, it.Name)
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
		"%s/%s/%s@sha256:%x", outregaddr, it.Namespace, it.Name, sha256.Sum256(manifest),
	), nil
}

// ImportTag runs an import on provided Tag.
func (i *Importer) ImportTag(
	ctx context.Context, it *imagtagv1.Tag,
) (imagtagv1.HashReference, error) {
	start := time.Now()

	var zero imagtagv1.HashReference
	if it.Spec.From == "" {
		i.metric.ReportImportFailure()
		return zero, fmt.Errorf("empty tag reference")
	}

	regDomain, remainder := i.SplitRegistryDomain(it.Spec.From)

	registries := i.syssvc.UnqualifiedRegistries(ctx)
	if regDomain != "" {
		registries = []string{regDomain}
	}
	if len(registries) == 0 {
		i.metric.ReportImportFailure()
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
				i.metric.ReportImportFailure()
				return zero, fmt.Errorf("error calculating digest: %w", err)
			}

			imageref := fmt.Sprintf("%s@%s", imgref.DockerReference().Name(), dgst)
			if it.Spec.Cache {
				imageref, err = i.cacheTag(ctx, it, imageref, sysctx)
				if err != nil {
					i.metric.ReportImportFailure()
					return zero, fmt.Errorf("unable to cache image: %w", err)
				}
			}

			i.metric.ReportImportSuccess()
			i.metric.ReportImportDuration(time.Since(start), it.Spec.Cache)

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

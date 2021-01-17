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
		return nil, fmt.Errorf("unable to parse docker reference: %w", err)
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
// cached image reference to be used. If tag has its cache flag set to
// false this is a no-op.
func (i *Importer) cacheTag(
	ctx context.Context,
	it *imagtagv1.Tag,
	from string,
	srcCtx *types.SystemContext,
) (string, error) {
	if !it.Spec.Cache {
		return from, nil
	}

	fromRef, err := i.ImageRefForStringRef(from)
	if err != nil {
		return "", fmt.Errorf("invalid source image reference: %w", err)
	}

	inregaddr, outregaddr, err := i.syssvc.CacheRegistryAddresses()
	if err != nil {
		return "", fmt.Errorf("unable to find cache registry: %w", err)
	}

	// We cache images under registry/namespace/image-tag.
	to := fmt.Sprintf("%s/%s/%s", inregaddr, it.Namespace, it.Name)
	toRef, err := i.ImageRefForStringRef(to)
	if err != nil {
		return "", fmt.Errorf("invalid destination image reference: %w", err)
	}

	polctx, err := i.DefaultPolicyContext()
	if err != nil {
		return "", fmt.Errorf("unable to get default policy: %w", err)
	}

	manifest, err := imgcopy.Image(
		ctx, polctx, toRef, fromRef, &imgcopy.Options{
			ImageListSelection: imgcopy.CopyAllImages,
			SourceCtx:          srcCtx,
			DestinationCtx:     i.syssvc.CacheRegistryContext(ctx),
		},
	)
	if err != nil {
		return "", fmt.Errorf("unable to copy image: %w", err)
	}

	return fmt.Sprintf(
		"%s/%s/%s@sha256:%x", outregaddr, it.Namespace, it.Name, sha256.Sum256(manifest),
	), nil
}

// registriesToSearch returns a list of registries to be used when looking for
// an image. It is either the provided domain or a list of unqualified domains
// configured globally and returned by SysContext service. This function is
// used when trying to understand what an user means when they simply ask to
// import an image called "centos:latest" for instance, in what registrise do
// we need to look for this image.
func (i *Importer) registriesToSearch(ctx context.Context, domain string) ([]string, error) {
	if domain != "" {
		return []string{domain}, nil
	}

	registries := i.syssvc.UnqualifiedRegistries(ctx)
	if len(registries) == 0 {
		return nil, fmt.Errorf("no unqualified registries found")
	}
	return registries, nil
}

// ImportTag runs an import on provided Tag. We look for the image in all
// configured unqualified registries using all authentications we can find
// on the Tag namespace.
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

	registries, err := i.registriesToSearch(ctx, regDomain)
	if err != nil {
		i.metric.ReportImportFailure()
		return zero, fmt.Errorf("fail to find source image domain: %w", err)
	}

	var errors *multierror.Error
	for _, registry := range registries {
		imgFullPath := fmt.Sprintf("%s/%s", registry, remainder)
		imgref, err := i.ImageRefForStringRef(imgFullPath)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		// get all authentications for registry and adds a nil
		// entry to the end in order to guarantee we gonna do
		// an import  attempt without using authentication.
		auths, err := i.syssvc.AuthsFor(ctx, imgref, it.Namespace)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}
		auths = append(auths, nil)

		for _, auth := range auths {
			sysctx := &types.SystemContext{
				DockerAuthConfig: auth,
			}

			imghash, err := i.getImageHash(ctx, sysctx, imgref)
			if err != nil {
				errors = multierror.Append(errors, err)
				continue
			}

			imghash, err = i.cacheTag(ctx, it, imghash, sysctx)
			if err != nil {
				i.metric.ReportImportFailure()
				return zero, fmt.Errorf("fail to cache image: %w", err)
			}

			i.metric.ReportImportSuccess()
			i.metric.ReportImportDuration(time.Since(start), it.Spec.Cache)

			return imagtagv1.HashReference{
				Generation:     it.Spec.Generation,
				From:           it.Spec.From,
				ImportedAt:     metav1.NewTime(time.Now()),
				ImageReference: imghash,
			}, nil
		}
	}

	i.metric.ReportImportFailure()
	return zero, fmt.Errorf("unable to import image: %w", errors)
}

// getImageHash attempts to fetch image hash remotely using provided system context.
func (i *Importer) getImageHash(
	ctx context.Context, sysctx *types.SystemContext, imgref types.ImageReference,
) (string, error) {
	img, err := imgref.NewImage(ctx, sysctx)
	if err != nil {
		return "", fmt.Errorf("unable to create ImageCloser: %w", err)
	}
	defer img.Close()

	manifestBlob, _, err := img.Manifest(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to fetch image manifest: %w", err)
	}

	dgst, err := manifest.Digest(manifestBlob)
	if err != nil {
		return "", fmt.Errorf("error calculating manifest digest: %w", err)
	}

	imageref := fmt.Sprintf("%s@%s", imgref.DockerReference().Name(), dgst)
	return imageref, nil
}

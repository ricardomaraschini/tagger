package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"

	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/hashicorp/go-multierror"

	"github.com/ricardomaraschini/tagger/infra/fs"
	"github.com/ricardomaraschini/tagger/infra/imagestore"
	imagtagv1 "github.com/ricardomaraschini/tagger/infra/tags/v1"
)

// Importer wrap srvices for tag import related operations.
type Importer struct {
	sync.Mutex
	istore *imagestore.ImageStore
	syssvc *SysContext
	fs     *fs.FS
}

// NewImporter returns a handler for tag related services. I have chosen to go
// with a lazy approach here, you can pass or omit (nil) the argument, it is
// up to the caller to decide what is needed for each specific case. So far this
// is the best approach, I still plan to review this.
func NewImporter(corinf informers.SharedInformerFactory) *Importer {
	return &Importer{
		syssvc: NewSysContext(corinf),
		fs:     fs.New(""),
	}
}

// getImageStore creates an instance of an ImageStore populated with our internal
// registry as storage. I have opted to do this here as it is every function here
// that needs to use an ImageStore instance and to restrict this whole struct to
// be used only when there is a cache registry is a no-go (i.e. when not caching
// images we don't need the ImageStore entity).
func (i *Importer) getImageStore(ctx context.Context) error {
	i.Lock()
	defer i.Unlock()
	if i.istore != nil {
		return nil
	}

	sysctx := i.syssvc.CacheRegistryContext(ctx)
	regaddr, _, err := i.syssvc.CacheRegistryAddresses()
	if err != nil {
		return fmt.Errorf("unable to discover cache registry: %w", err)
	}

	defpol, err := i.syssvc.DefaultPolicyContext()
	if err != nil {
		return fmt.Errorf("error reading default policy: %w", err)
	}

	i.istore = imagestore.NewImageStore(regaddr, sysctx, defpol)
	return nil
}

// splitRegistryDomain splits the domain from the repository and image.
// For example passing in the "quay.io/tagger/tagger:latest" string will
// result in returned values "quay.io" and "tagger:tagger:latest".
func (i *Importer) splitRegistryDomain(imgPath string) (string, string) {
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

// ImportTag runs an import on provided Tag. By Import here we mean to discover
// what is the current hash for a given image in a given tag. We look for the image
// in all configured unqualified registries using all authentications we can find
// in the Tag namespace. If the tag is set to be cached (spec.cache = true) we
// push the image to our cache registry.
func (i *Importer) ImportTag(
	ctx context.Context, it *imagtagv1.Tag,
) (imagtagv1.HashReference, error) {
	var zero imagtagv1.HashReference
	if it.Spec.From == "" {
		return zero, fmt.Errorf("empty tag reference")
	}
	domain, remainder := i.splitRegistryDomain(it.Spec.From)

	registries, err := i.registriesToSearch(ctx, domain)
	if err != nil {
		return zero, fmt.Errorf("fail to find source image domain: %w", err)
	}

	var errors *multierror.Error
	for _, registry := range registries {
		imgpath := fmt.Sprintf("docker://%s/%s", registry, remainder)
		imgref, err := alltransports.ParseImageName(imgpath)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		syscontexts, err := i.syssvc.SystemContextsFor(ctx, imgref, it.Namespace)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		imghash, sysctx, err := i.istore.GetImageTagHash(ctx, imgref, syscontexts)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		if it.Spec.Cache {
			if err := i.getImageStore(ctx); err != nil {
				return zero, fmt.Errorf("unable to get image store: %w", err)
			}

			if imghash, err = i.istore.Load(
				ctx, imghash, sysctx, it.Namespace, it.Name,
			); err != nil {
				return zero, fmt.Errorf("fail to cache image: %w", err)
			}
		}

		return imagtagv1.HashReference{
			Generation:     it.Spec.Generation,
			From:           it.Spec.From,
			ImportedAt:     metav1.NewTime(time.Now()),
			ImageReference: imghash.DockerReference().String(),
		}, nil
	}

	return zero, fmt.Errorf("unable to import image: %w", errors)
}

// LoadTagImage loads an image into our cache registry.
func (i *Importer) LoadTagImage(
	ctx context.Context,
	from types.ImageReference,
	fromCtx *types.SystemContext,
	repo string,
	name string,
) (string, error) {
	if err := i.getImageStore(ctx); err != nil {
		return "", fmt.Errorf("error creating image store: %w", err)
	}

	ref, err := i.istore.Load(ctx, from, fromCtx, repo, name)
	if err != nil {
		return "", fmt.Errorf("error loading image into registry: %w", err)
	}

	return ref.DockerReference().String(), nil
}

// SaveTagImage pulls current generation of a given Tag into a local tar file.
func (i *Importer) SaveTagImage(
	ctx context.Context, it *imagtagv1.Tag,
) (string, func(), error) {
	if err := i.getImageStore(ctx); err != nil {
		return "", nil, fmt.Errorf("error creating image store: %w", err)
	}

	imgref := it.CurrentReferenceForTag()
	if len(imgref) == 0 {
		return "", nil, fmt.Errorf("reference for current generation not found")
	}

	from := fmt.Sprintf("docker://%s", imgref)
	fromRef, err := alltransports.ParseImageName(from)
	if err != nil {
		return "", nil, fmt.Errorf("error parsing image reference: %w", err)
	}

	toRef, cleanup, err := i.istore.Save(ctx, fromRef)
	if err != nil {
		return "", nil, fmt.Errorf("error saving image locally: %w", err)
	}

	return toRef.StringWithinTransport(), cleanup, nil
}

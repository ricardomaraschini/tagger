package services

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"

	imgcopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/directory"
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
// registry as storage. I have opted to do this here as it is better not to
// full our New() functions with this kind of thing.
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

// ImportTag runs an import on provided Tag. We look for the image in all configured
// unqualified registries using all authentications we can find on the Tag namespace.
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

		imghash, sysctx, err := i.istore.GetImageHash(ctx, imgref, syscontexts)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		if it.Spec.Cache {
			if err := i.getImageStore(ctx); err != nil {
				return zero, fmt.Errorf("unable to get registry reference: %w", err)
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

// LoadTagImage loads an image into our cache registry and updates provided tag to
// points to its highest Generation to the new pushed image. If we fail during the
// second operation that is ok, client will retry and the blobs will be already
// present in the cache registry.
func (i *Importer) LoadTagImage(
	ctx context.Context,
	from types.ImageReference,
	fromCtx *types.SystemContext,
	repo string,
	name string,
) (string, error) {
	inregaddr, outregaddr, err := i.syssvc.CacheRegistryAddresses()
	if err != nil {
		return "", fmt.Errorf("unable to find cache registry: %w", err)
	}
	to := fmt.Sprintf("%s/%s/%s", inregaddr, repo, name)
	toRef, err := i.ImageRefForStringRef(to)
	if err != nil {
		return "", fmt.Errorf("invalid destination image reference: %w", err)
	}

	polctx, err := i.syssvc.DefaultPolicyContext()
	if err != nil {
		return "", fmt.Errorf("unable to get default policy: %w", err)
	}

	manifest, err := imgcopy.Image(
		ctx, polctx, toRef, from, &imgcopy.Options{
			ImageListSelection: imgcopy.CopyAllImages,
			SourceCtx:          fromCtx,
			DestinationCtx:     i.syssvc.CacheRegistryContext(ctx),
		},
	)
	if err != nil {
		return "", fmt.Errorf("unable to copy image: %w", err)
	}

	return fmt.Sprintf(
		"%s/%s/%s@sha256:%x", outregaddr, repo, name, sha256.Sum256(manifest),
	), nil
}

// pushImageFromDir reads an image from a directory and pushes it to the
// cache registry. This uses the cache registry context, do not try to
// use this to push images to other registries.
func (i *Importer) pushImageFromDir(
	ctx context.Context, dir string, toRef types.ImageReference,
) error {
	fromRef, err := directory.NewReference(dir)
	if err != nil {
		return fmt.Errorf("unable to create dir reference: %w", err)
	}

	polctx, err := i.syssvc.DefaultPolicyContext()
	if err != nil {
		return fmt.Errorf("unable to get default policy: %w", err)
	}

	if _, err := imgcopy.Image(
		ctx, polctx, toRef, fromRef, &imgcopy.Options{
			ImageListSelection: imgcopy.CopyAllImages,
			DestinationCtx:     i.syssvc.CacheRegistryContext(ctx),
		},
	); err != nil {
		return fmt.Errorf("error pushing image from disk: %w", err)
	}
	return nil
}

// SaveTagImage pulls current generation of a given Tag into a local tar file.
// Pull progress is reported through provided progress channel, AFAIK we don't
// close the provided channel at the end of progress it seems :-(.
func (i *Importer) SaveTagImage(
	ctx context.Context,
	it *imagtagv1.Tag,
	dst string,
	progress chan types.ProgressProperties,
) error {
	imgref := it.CurrentReferenceForTag()
	if len(imgref) == 0 {
		return fmt.Errorf("reference for current generation not found")
	}

	fromRef, err := i.ImageRefForStringRef(imgref)
	if err != nil {
		return fmt.Errorf("error parsing image reference: %w", err)
	}

	dstref := fmt.Sprintf("docker-archive:%s", dst)
	toRef, err := alltransports.ParseImageName(dstref)
	if err != nil {
		return fmt.Errorf("error creating dir reference: %w", err)
	}

	polctx, err := i.syssvc.DefaultPolicyContext()
	if err != nil {
		return fmt.Errorf("unable to get default policy: %w", err)
	}

	auths, err := i.syssvc.AuthsFor(ctx, fromRef, it.Namespace)
	if err != nil {
		return fmt.Errorf("error reading docker secrets: %w", err)
	}
	// by adding a nil entry at the end of the slice we assure that we are
	// going to give "no credentials" access a try.
	auths = append(auths, nil)

	var errors *multierror.Error
	for _, auth := range auths {
		sysctx := &types.SystemContext{
			DockerAuthConfig: auth,
		}

		_, err := imgcopy.Image(
			ctx, polctx, toRef, fromRef, &imgcopy.Options{
				SourceCtx:        sysctx,
				ProgressInterval: 500 * time.Millisecond,
				Progress:         progress,
			},
		)
		if err == nil {
			return nil
		}
		errors = multierror.Append(errors, err)
	}
	return fmt.Errorf("unable to pull tag image: %w", errors)
}

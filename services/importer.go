package services

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"

	imgcopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/directory"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	"github.com/hashicorp/go-multierror"

	"github.com/ricardomaraschini/tagger/infra/fs"
	imagtagv1 "github.com/ricardomaraschini/tagger/infra/tags/v1"
)

// Importer wrap srvices for tag import related operations.
type Importer struct {
	syssvc *SysContext
	metric *Metric
	fs     *fs.FS
}

// NewImporter returns a handler for tag related services. I have chosen to go
// with a lazy approach here, you can pass or omit (nil) the argument, it is
// up to the caller to decide what is needed for each specific case. So far this
// is the best approach, I still plan to review this.
func NewImporter(corinf informers.SharedInformerFactory) *Importer {
	return &Importer{
		syssvc: NewSysContext(corinf),
		metric: NewMetrics(),
		fs:     fs.New("/data"),
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

	polctx, err := i.syssvc.DefaultPolicyContext()
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

// PushTagFromDir tag from dir reads a tag from a directory and pushes images that
// refer to the cache registry.
func (i *Importer) PushTagFromDir(ctx context.Context, it *imagtagv1.Tag, dir string) error {
	inregaddr, _, err := i.syssvc.CacheRegistryAddresses()
	if err != nil {
		return fmt.Errorf("unable to find cache registry: %w", err)
	}

	blobsdir := fmt.Sprintf("%s/blobs", dir)
	for _, hr := range it.Status.References {
		domain, _ := i.SplitRegistryDomain(hr.ImageReference)
		if domain != inregaddr {
			continue
		}

		ref, err := i.ImageRefForStringRef(hr.ImageReference)
		if err != nil {
			return fmt.Errorf("error parsing generation: %w", err)
		}

		for _, fname := range []string{
			"manifest.json",
			"version",
		} {
			oname := fmt.Sprintf("%s/%d-%s", dir, hr.Generation, fname)
			nname := fmt.Sprintf("%s/%s", blobsdir, fname)
			if err := os.Rename(oname, nname); err != nil {
				return fmt.Errorf("error moving file %s: %s", oname, err)
			}
		}

		if err := i.pushImageFromDir(ctx, blobsdir, ref); err != nil {
			return fmt.Errorf("error pulling image: %w", err)
		}
	}
	return nil
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

// PullTagToDir pull all Tag's generations hosted locally (cached) and stores them
// into a directory. Inside the resulting directory every generation will be placed
// in a different subdirectory, subdirs are named after their generation number.
// Pull progress is reported through the progress channel.
func (i *Importer) PullTagToDir(
	ctx context.Context,
	it *imagtagv1.Tag,
	dstdir string,
	progress chan types.ProgressProperties,
) error {
	inregaddr, _, err := i.syssvc.CacheRegistryAddresses()
	if err != nil {
		return fmt.Errorf("unable to find cache registry: %w", err)
	}

	tmpdir, cleanup, err := i.fs.TempDir()
	if err != nil {
		return fmt.Errorf("error creating temp dir: %w", err)
	}
	defer cleanup()

	blobsdir := fmt.Sprintf("%s/blobs", dstdir)
	if err := os.Mkdir(blobsdir, 0700); err != nil {
		return fmt.Errorf("error creating blobs dir: %w", err)
	}

	for _, hr := range it.Status.References {
		domain, _ := i.SplitRegistryDomain(hr.ImageReference)
		if domain != inregaddr {
			continue
		}

		ref, err := i.ImageRefForStringRef(hr.ImageReference)
		if err != nil {
			return fmt.Errorf("error parsing generation: %w", err)
		}

		if err := i.pullImageToDir(ctx, ref, tmpdir, progress); err != nil {
			return fmt.Errorf("error pulling image: %w", err)
		}

		// move manifest and version files out of the temporary dir
		// and into our definitive directory (provided through args
		// to this function).
		for _, fname := range []string{
			"manifest.json",
			"version",
		} {
			oname := fmt.Sprintf("%s/%s", tmpdir, fname)
			nname := fmt.Sprintf("%s/%d-%s", dstdir, hr.Generation, fname)
			if err := os.Rename(oname, nname); err != nil {
				return fmt.Errorf("error moving file %s: %s", oname, err)
			}
		}

		// now move all the rest of the files from temporary dir
		// into their final destionation, as we already moved
		// manifest and version files only blobs must be residing
		// on the temporary dir.
		if err := i.fs.MoveFiles(tmpdir, blobsdir); err != nil {
			return fmt.Errorf("error moving blobs: %w", err)
		}
	}
	return nil
}

// pullImageToDir pulls an image hosted in the cache registry into a local
// directory. This function uses the cache registry context so it is not
// suitable to pull image from other registries.
func (i *Importer) pullImageToDir(
	ctx context.Context,
	fromRef types.ImageReference,
	dir string,
	progress chan types.ProgressProperties,
) error {
	toRef, err := directory.NewReference(dir)
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
			SourceCtx:          i.syssvc.CacheRegistryContext(ctx),
			ProgressInterval:   500 * time.Millisecond,
			Progress:           progress,
		},
	); err != nil {
		return fmt.Errorf("error pulling image to disk: %w", err)
	}
	return nil
}

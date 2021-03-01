package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/hashicorp/go-multierror"

	"github.com/ricardomaraschini/tagger/infra/fs"
	imagtagv1 "github.com/ricardomaraschini/tagger/infra/tags/v1"
)

// Importer wrap services for tag import related operations.
type Importer struct {
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

// splitRegistryDomain splits the domain from the repository and image.
// For example passing in the "quay.io/tagger/tagger:latest" string will
// result in returned values "quay.io" and "tagger/tagger:latest".
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

// ImportTag runs an import on provided Tag. By Import here we mean to discover
// what is the current hash for a given image in a given tag. We look for the image
// in all configured unqualified registries using all authentications we can find
// for the registry in the Tag namespace. If the tag is set to be cached we push the
// image to our cache registry.
func (i *Importer) ImportTag(
	ctx context.Context, it *imagtagv1.Tag,
) (imagtagv1.HashReference, error) {
	var zero imagtagv1.HashReference
	if it.Spec.From == "" {
		return zero, fmt.Errorf("empty tag reference")
	}
	domain, remainder := i.splitRegistryDomain(it.Spec.From)

	registries, err := i.syssvc.RegistriesToSearch(ctx, domain)
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

		imghash, sysctx, err := i.GetImageTagHash(ctx, imgref, syscontexts)
		if err != nil {
			errors = multierror.Append(errors, err)
			continue
		}

		if it.Spec.Cache {
			istore, err := i.syssvc.GetRegistryStore(ctx)
			if err != nil {
				return zero, fmt.Errorf("unable to get image store: %w", err)
			}

			if imghash, err = istore.Load(
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

// getImageTagHash attempts to fetch image hash remotely using provided system
// context.  By image hash I mean the full image path with its hash, something
// like: quay.io/tagger/tagger@sha256:... The ideia here is that the "from"
// reference points to a image by tag (e.g. quay.io/tagger/taggger:latest).
func (i *Importer) getImageTagHash(
	ctx context.Context, from types.ImageReference, sysctx *types.SystemContext,
) (types.ImageReference, error) {
	img, err := from.NewImage(ctx, sysctx)
	if err != nil {
		return nil, fmt.Errorf("unable to create image closer: %w", err)
	}
	defer img.Close()

	manifestBlob, _, err := img.Manifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch image manifest: %w", err)
	}

	dgst, err := manifest.Digest(manifestBlob)
	if err != nil {
		return nil, fmt.Errorf("error calculating manifest digest: %w", err)
	}

	refstr := fmt.Sprintf("docker://%s@%s", from.DockerReference().Name(), dgst)
	hashref, err := alltransports.ParseImageName(refstr)
	if err != nil {
		return nil, err
	}
	return hashref, nil
}

// GetImageTagHash attempts to obtain the hash for a given image on a remote registry.
// It runs through provided system contexts trying all of them. If no SystemContext
// is present it does one attemp without authentication. Returns the image reference
// and the SystemContext that worked (the one whose credentials work) or an error.
func (i *Importer) GetImageTagHash(
	ctx context.Context, imgref types.ImageReference, sysctxs []*types.SystemContext,
) (types.ImageReference, *types.SystemContext, error) {
	// if no contexts then we do an attempt without using any credentials.
	if len(sysctxs) == 0 {
		sysctxs = []*types.SystemContext{nil}
	}

	var errors *multierror.Error
	for _, sysctx := range sysctxs {
		imghash, err := i.getImageTagHash(ctx, imgref, sysctx)
		if err == nil {
			return imghash, sysctx, nil
		}
		errors = multierror.Append(errors, err)
	}
	return nil, nil, fmt.Errorf("unable to get hash for image tag: %w", errors)
}

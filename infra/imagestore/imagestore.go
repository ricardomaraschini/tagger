package imagestore

import (
	"context"
	"fmt"

	imgcopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/hashicorp/go-multierror"

	"github.com/ricardomaraschini/tagger/infra/fs"
)

// ImageStore wraps cals for iteracting with our backend registry.
type ImageStore struct {
	fs      *fs.FS
	regaddr string
	polctx  *signature.PolicyContext
	regctx  *types.SystemContext
}

// NewImageStore creates an entity capable of load objects to or save objects
// from from a backend registry. When calling Load we push an image to the
// registry, when calling Save we pull the image from the registry and store
// into a local tar file. XXX This entity should be reviewed, adding cache
// or even change the way we store images locally on save may be needed.
func NewImageStore(
	regaddr string,
	sysctx *types.SystemContext,
	polctx *signature.PolicyContext,
) *ImageStore {
	return &ImageStore{
		fs:      fs.New(""),
		regaddr: regaddr,
		polctx:  polctx,
		regctx:  sysctx,
	}
}

// Load pushes an image into the backend registry into namespace/name index.
// Uses srcctx (of type types.SystemContext) when reading image from srcref,
// so when copying from one remote registry into our cache registry srcctx
// must contain all needed authentication information.
func (i *ImageStore) Load(
	ctx context.Context,
	srcref types.ImageReference,
	srcctx *types.SystemContext,
	repo string,
	name string,
) (types.ImageReference, error) {
	tostr := fmt.Sprintf("docker://%s/%s/%s", i.regaddr, repo, name)
	toref, err := alltransports.ParseImageName(tostr)
	if err != nil {
		return nil, fmt.Errorf("invalid destination reference: %w", err)
	}

	manblob, err := imgcopy.Image(
		ctx, i.polctx, toref, srcref, &imgcopy.Options{
			ImageListSelection: imgcopy.CopyAllImages,
			SourceCtx:          srcctx,
			DestinationCtx:     i.regctx,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("unable to load image: %w", err)
	}

	dgst, err := manifest.Digest(manblob)
	if err != nil {
		return nil, fmt.Errorf("error calculating manifest digest: %w", err)
	}

	refstr := fmt.Sprintf("docker://%s@%s", toref.DockerReference().Name(), dgst)
	return alltransports.ParseImageName(refstr)
}

// Save pulls an image from our cache registry, stores it in a temporary tar
// file on disk.  Returns an ImageReference pointing to the local tar file
// and a function the caller needs to call in order to clean up (property
// close tar file and delete it from disk).
func (i *ImageStore) Save(
	ctx context.Context, ref types.ImageReference,
) (types.ImageReference, func(), error) {
	dstref, cleanup, err := i.NewLocalReference()
	if err != nil {
		return nil, nil, fmt.Errorf("error creating temp file: %w", err)
	}

	_, err = imgcopy.Image(
		ctx, i.polctx, dstref, ref, &imgcopy.Options{
			SourceCtx: i.regctx,
		},
	)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("unable to copy image: %w", err)
	}
	return dstref, cleanup, nil
}

// getImageTagHash attempts to fetch image hash remotely using provided system
// context.  By image hash I mean the full image path with its hash, something
// like: quay.io/tagger/tagger@sha256:... The ideia here is that the "from"
// reference points to a image by tag (e.g. quay.io/tagger/taggger:latest).
func (i *ImageStore) getImageTagHash(
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
func (i *ImageStore) GetImageTagHash(
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

// NewLocalReference returns an image reference pointing to a local tar file. Also
// returns a clean up function that must be called to free resources.
func (i *ImageStore) NewLocalReference() (types.ImageReference, func(), error) {
	tfile, cleanup, err := i.fs.TempFile()
	if err != nil {
		return nil, nil, fmt.Errorf("error creating temp file: %w", err)
	}
	fpath := fmt.Sprintf("docker-archive:%s", tfile.Name())
	ref, err := alltransports.ParseImageName(fpath)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("error creating new local ref: %w", err)
	}
	return ref, cleanup, nil
}

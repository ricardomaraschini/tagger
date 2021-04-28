package imagestore

import (
	"context"
	"fmt"

	imgcopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"

	"github.com/ricardomaraschini/tagger/infra/fs"
)

// CleanFunc is a function that must be called in order to clean up
// or free resources in use.
type CleanFunc func()

// Registry wraps calls for iteracting with our backend registry. It
// provides an implementation capable of pushing to and pulling from
// an image registry. To push an image towards the registry one needs
// to call Load, to push it to a local tar file a Save call should be
// made, this strange naming is an attempt to make it similar to the
// 'docker save/load' commands.
type Registry struct {
	fs      *fs.FS
	regaddr string
	polctx  *signature.PolicyContext
	regctx  *types.SystemContext
}

// NewRegistry creates an entity capable of load objects to or save
// objects from a backend registry. When calling Load we push an image
// into the registry, when calling Save we pull the image from the
// registry and store into a local tar file (format in disk is of type
// docker-archive, we should migrate this to something else as it does
// not support manifest lists).
func NewRegistry(
	regaddr string,
	sysctx *types.SystemContext,
	polctx *signature.PolicyContext,
) *Registry {
	return &Registry{
		fs:      fs.New(""),
		regaddr: regaddr,
		polctx:  polctx,
		regctx:  sysctx,
	}
}

// Load pushes an image reference into the backend registry using repo/name
// as its destination. Uses srcctx (of type types.SystemContext) when reading
// image from srcref, so when copying from one remote registry into our mirror
// registry srcctx must contain all needed authentication information.
func (i *Registry) Load(
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

// Save pulls an image from our mirror registry, stores it in a temporary
// tar file on disk.  Returns an ImageReference pointing to the local tar
// file and a function the caller needs to call in order to clean up after
// our mess (properly close tar file and delete it from disk). Returned ref
// points to a 'docker-archive' tar file.
func (i *Registry) Save(
	ctx context.Context, ref types.ImageReference,
) (types.ImageReference, CleanFunc, error) {
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

// NewLocalReference returns an image reference pointing to a local tar file.
// Also returns a clean up function that must be called to free resources.
func (i *Registry) NewLocalReference() (types.ImageReference, CleanFunc, error) {
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

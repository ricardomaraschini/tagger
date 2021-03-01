package services

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"

	"github.com/containers/image/v5/transports/alltransports"

	imagtagv1 "github.com/ricardomaraschini/tagger/infra/tags/v1"
	tagclient "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/clientset/versioned"
	taginform "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/informers/externalversions"
	taglist "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/listers/tags/v1"
)

// TagIO is an entity that gather operations related to Tag images input and
// output. This entity allow users to pull images from or to push images to
// a Tag.
type TagIO struct {
	tagcli tagclient.Interface
	taglis taglist.TagLister
	syssvc *SysContext
}

// NewTagIO returns a new TagIO object, capable of import and export Tag images.
func NewTagIO(
	corinf informers.SharedInformerFactory,
	tagcli tagclient.Interface,
	taginf taginform.SharedInformerFactory,
) *TagIO {
	var taglis taglist.TagLister
	if taginf != nil {
		taglis = taginf.Images().V1().Tags().Lister()
	}

	return &TagIO{
		tagcli: tagcli,
		taglis: taglis,
		syssvc: NewSysContext(corinf),
	}
}

// tagOrNew returns or an existing Tag or a new one.
func (t *TagIO) tagOrNew(ns, name string) (*imagtagv1.Tag, error) {
	it, err := t.taglis.Tags(ns).Get(name)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}

	if err == nil {
		return it, nil
	}

	return &imagtagv1.Tag{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: imagtagv1.TagSpec{
			Cache: true,
		},
	}, nil
}

// Push expects "fpath" to point to a valid docker image stored on disk as a tar
// file, reads it and then pushes it to our cache registry through an image store
// implementation (see infra/imagestore/registry.go).
func (t *TagIO) Push(ctx context.Context, ns, name string, fpath string) error {
	istore, err := t.syssvc.GetRegistryStore(ctx)
	if err != nil {
		return fmt.Errorf("error creating image store: %w", err)
	}

	it, err := t.tagOrNew(ns, name)
	if err != nil {
		return fmt.Errorf("error getting tag: %w", err)
	}

	refstr := fmt.Sprintf("docker-archive://%s", fpath)
	srcref, err := alltransports.ParseImageName(refstr)
	if err != nil {
		return fmt.Errorf("error parsing image name: %w", err)
	}

	// we pass nil as source context reference as to read the file from disk
	// no authentication is needed. Namespace is used as repository and
	// name as image name.
	dstref, err := istore.Load(ctx, srcref, nil, ns, name)
	if err != nil {
		return fmt.Errorf("error loading image into registry: %w", err)
	}

	nextgen := it.NextGeneration()
	it.Spec.Generation, it.Status.Generation = nextgen, nextgen
	it.RegisterImportSuccess()
	it.PrependHashReference(
		imagtagv1.HashReference{
			Generation:     nextgen,
			From:           "-",
			ImportedAt:     metav1.Now(),
			ImageReference: dstref.DockerReference().String(),
		},
	)

	// that means we have received a push to a tag that does not
	// exist yet, in such scenario there is no possible "from" to
	// use on its spec so we set it to the place where we pushed
	// the image.
	if it.Spec.From == "" {
		it.Spec.From = dstref.DockerReference().String()
		_, err = t.tagcli.ImagesV1().Tags(ns).Create(
			ctx, it, metav1.CreateOptions{},
		)
		return err
	}

	_, err = t.tagcli.ImagesV1().Tags(ns).Update(
		ctx, it, metav1.UpdateOptions{},
	)
	return err
}

// Pull saves a Tag image into a tar file and returns a reader from where the image
// content can be read. Caller is responsible for cleaning up after the returned
// resources by calling the returned function.
func (t *TagIO) Pull(ctx context.Context, ns, name string) (*os.File, func(), error) {
	it, err := t.taglis.Tags(ns).Get(name)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting tag: %w", err)
	}

	istore, err := t.syssvc.GetRegistryStore(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating image store: %w", err)
	}

	imgref := it.CurrentReferenceForTag()
	if len(imgref) == 0 {
		return nil, nil, fmt.Errorf("reference for current generation not found")
	}

	from := fmt.Sprintf("docker://%s", imgref)
	fromRef, err := alltransports.ParseImageName(from)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing image reference: %w", err)
	}

	toRef, cleanup, err := istore.Save(ctx, fromRef)
	if err != nil {
		return nil, nil, fmt.Errorf("error saving image locally: %w", err)
	}
	fpath := toRef.StringWithinTransport()

	fp, err := os.Open(fpath)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("error opening tar file: %w", err)
	}
	ncleanup := func() {
		fp.Close()
		cleanup()
	}
	return fp, ncleanup, nil
}

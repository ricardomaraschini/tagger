package services

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"

	"github.com/containers/image/v5/transports/alltransports"

	imagtagv1 "github.com/ricardomaraschini/tagger/infra/tags/v1"
	tagclient "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/clientset/versioned"
	taginform "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/informers/externalversions"
	taglist "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/listers/tags/v1"
)

// TagIO is an entity that gather operations related to Tag images input and
// output. This entity allow users to pull from or to push to a Tag.
type TagIO struct {
	tagcli tagclient.Interface
	taglis taglist.TagLister
	impsvc *Importer
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
		impsvc: NewImporter(corinf),
	}
}

// Push reads a compressed tag from argument "from" and pushes it to our
// registry storage. If success it then updates the tag accordingly to
// indicate the new Generation.
func (t *TagIO) Push(
	ctx context.Context, ns, name string, fpath string,
) error {
	it, err := t.taglis.Tags(ns).Get(name)
	if err != nil {
		return err
	}

	refstr := fmt.Sprintf("docker-archive://%s", fpath)
	srcref, err := alltransports.ParseImageName(refstr)
	if err != nil {
		return fmt.Errorf("error parsing image name: %w", err)
	}

	pushedTo, err := t.impsvc.LoadTagImage(ctx, srcref, nil, ns, name)
	if err != nil {
		return fmt.Errorf("error loading image into cache registry: %w", err)
	}

	nextgen := it.NextGeneration()
	it.Spec.Generation = nextgen
	it.Status.Generation = nextgen
	it.RegisterImportSuccess()
	it.PrependHashReference(
		imagtagv1.HashReference{
			Generation:     nextgen,
			From:           "-",
			ImportedAt:     metav1.Now(),
			ImageReference: pushedTo,
		},
	)

	if _, err := t.tagcli.ImagesV1().Tags(ns).Update(
		ctx, it, metav1.UpdateOptions{},
	); err != nil {
		return fmt.Errorf("error updating tag object: %w", err)
	}
	return nil
}

// Pull saves a Tag iamge into a tar file and returns a reader from where
// the image content can be read. Caller is responsible for cleaning up
// after the returned value by calling the function we return (2nd return),
// by issuing a call to the returned func() the tar will be closed and
// deleted from disk.
func (t *TagIO) Pull(
	ctx context.Context, ns string, name string,
) (*os.File, func(), error) {
	it, err := t.taglis.Tags(ns).Get(name)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting tag: %w", err)
	}

	fpath, cleanup, err := t.impsvc.SaveTagImage(ctx, it)
	if err != nil {
		return nil, nil, fmt.Errorf("error pulling tag to dir: %w", err)
	}

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

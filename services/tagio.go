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

// newTag returns a new tag object as if it is being pushed by the client.
// From propert is set to stdin ("-") and the Tag is set as cacheable.
func (t *TagIO) newTag(name string) *imagtagv1.Tag {
	return &imagtagv1.Tag{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: imagtagv1.TagSpec{
			From:  "-",
			Cache: true,
		},
	}
}

// tagOrNew returns or an existing tag or a new one. If the tag exists
// returns it otherwise creates a new one.
func (t *TagIO) tagOrNew(ns, name string) (*imagtagv1.Tag, error) {
	it, err := t.taglis.Tags(ns).Get(name)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	if errors.IsNotFound(err) {
		return t.newTag(name), nil
	}
	return it, nil
}

// Push reads a compressed tag from argument "from" and pushes it to our
// registry storage. If success it then updates the tag accordingly to
// indicate the new Generation.
func (t *TagIO) Push(
	ctx context.Context, ns, name string, fpath string,
) error {
	it, err := t.tagOrNew(ns, name)
	if err != nil {
		return fmt.Errorf("error getting tag: %w", err)
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

	// from this point on it is an Upsert logic.
	if it.ObjectMeta.GetResourceVersion() == "" {
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

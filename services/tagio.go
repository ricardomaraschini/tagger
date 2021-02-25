package services

import (
	"context"
	"fmt"
	"io"
	"os"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/informers"
	"k8s.io/klog/v2"

	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/ricardomaraschini/tagger/infra/fs"
	tagclient "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/clientset/versioned"
	taginform "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/informers/externalversions"
	taglist "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/listers/tags/v1"
)

// TagIO is an entity that gather operations related to Tag input/output.
// Input and output is related to a cluster, we allow users to input (import
// a tag from a different cluster) and output (export a tag to a different
// cluster. For this entity Pull and Push are functions to be interpreted
// from the client's point of view, i.e. pull means a client pulling a tag
// from tagger while push means a client pushing a tag to tagger.
type TagIO struct {
	tagcli tagclient.Interface
	taglis taglist.TagLister
	syssvc *SysContext
	impsvc *Importer
	fstsvc *fs.FS
}

// NewTagIO returns a new TagIO object, capable of import and export Tags.
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
		impsvc: NewImporter(corinf),
		fstsvc: fs.New("/data"),
	}
}

// Push reads a compressed tag from argument "from", uncompress it into a
// temporary directory and attempts to create a tag based on its content,
// returns an error if the tag already exists.
func (t *TagIO) Push(
	ctx context.Context, ns, name string, from io.Reader,
) error {
	it, err := t.taglis.Tags(ns).Get(name); err == nil {
		return fmt.Errorf("unable to import tag, already exists")
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("error checking for tag existence: %w", err)
	}

	tmpfile, cleanup, err := t.fstsvc.TempFile()
	if err != nil {
		return fmt.Errorf("unable to create temp dir: %w", err)
	}
	defer cleanup()
	fname := tmpfile.Name()

	if _, err := io.Copy(tmpfile, from); err != nil {
		return fmt.Errorf("error copying streams: %w", err)
	}

	srcref := fmt.Sprintf("docker-archive:%s", fname)
	fromRef, err := alltransports.ParseImageName(srcref)
	if err != nil {
		return fmt.Errorf("error creating source ref: %w", err)
	}

	pushedTo, err := t.impsvc.LoadTagImage(ctx, fromRef, nil, ns, name)
	if err != nil {
		return fmt.Errorf("error loading image into cache registry: %w", err)
	}



	klog.Infof(pushedTo)
	return nil
}

// Pull saves a Tag into a local tar file and returns a reader closer to
// it.  Caller is responsible for cleaning up after the returned value by
// calling the function we return (2nd return), by deferring a call to the
// returned func() the tar will be closed and deleted from disk.
func (t *TagIO) Pull(
	ctx context.Context, ns string, name string, progress chan types.ProgressProperties,
) (*os.File, func(), error) {
	it, err := t.taglis.Tags(ns).Get(name)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting tag: %w", err)
	}

	fp, cleanfile, err := t.fstsvc.TempFile()
	if err != nil {
		return nil, nil, fmt.Errorf("error creating tar file: %w", err)
	}
	fname := fp.Name()
	fp.Close()

	if err := t.impsvc.SaveTagImage(ctx, it, fname, progress); err != nil {
		cleanfile()
		return nil, nil, fmt.Errorf("error pulling tag to dir: %w", err)
	}

	tar, err := os.Open(fname)
	if err != nil {
		cleanfile()
		return nil, nil, fmt.Errorf("unable to open tar file: %w", err)
	}
	return tar, cleanfile, nil
}

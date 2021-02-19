package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"

	"github.com/containers/image/v5/types"
	"github.com/ricardomaraschini/tagger/infra/fs"
	imagtagv1 "github.com/ricardomaraschini/tagger/infra/tags/v1"
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
	if _, err := t.taglis.Tags(ns).Get(name); err == nil {
		return fmt.Errorf("unable to import tag, already exists")
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("error checking for tag existence: %w", err)
	}

	tmpdir, cleanup, err := t.fstsvc.TempDir()
	if err != nil {
		return fmt.Errorf("unable to create temp dir: %s", err)
	}
	defer cleanup()

	if err := t.fstsvc.UnarchiveFile(from, tmpdir); err != nil {
		return fmt.Errorf("error decompressing tag: %s", err)
	}

	tagfp := fmt.Sprintf("%s/tag.json", tmpdir)
	it, err := t.fillAndDecodeTag(tagfp, ns, name)
	if err != nil {
		return fmt.Errorf("unable to parse tag for import: %w", err)
	}

	if err := t.impsvc.PushTagFromDir(ctx, it, tmpdir); err != nil {
		return fmt.Errorf("error pushing tag from dir: %w", err)
	}

	if _, err := t.tagcli.ImagesV1().Tags(ns).Create(
		ctx, it, metav1.CreateOptions{},
	); err != nil {
		return fmt.Errorf("unable to save tag: %w", err)
	}
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

	dir, cleandir, err := t.fstsvc.TempDir()
	if err != nil {
		return nil, nil, fmt.Errorf("error creating temp dir: %w", err)
	}
	defer cleandir()

	if err := t.impsvc.PullTagToDir(ctx, it, dir, progress); err != nil {
		return nil, nil, fmt.Errorf("error pulling tag to dir: %w", err)
	}

	if err := t.cleanAndEncodeTag(it, dir); err != nil {
		return nil, nil, fmt.Errorf("error encoding tag: %w", err)
	}

	tar, cleanfile, err := t.fstsvc.TempFile()
	if err != nil {
		return nil, nil, fmt.Errorf("error creating tar file: %w", err)
	}

	if err := t.fstsvc.ArchiveDirectory(dir, tar); err != nil {
		cleanfile()
		return nil, nil, fmt.Errorf("error compressing tag: %w", err)
	}

	// make sure we rewind the file
	if _, err := tar.Seek(0, 0); err != nil {
		cleanfile()
		return nil, nil, fmt.Errorf("error on tar seek op: %w", err)
	}

	return tar, cleanfile, nil
}

// fillAndDecodeTag undo what is done by cleanAndEncodeTag, it receives a file
// containing a tag with some template entries (.Namespace, .Name and .Registry),
// parses the template to replace these entries and then unmarshals it, returning
// the Tag object or an error.
func (t *TagIO) fillAndDecodeTag(fpath, ns, name string) (*imagtagv1.Tag, error) {
	inregaddr, _, err := t.syssvc.CacheRegistryAddresses()
	if err != nil {
		return nil, fmt.Errorf("fail to get cache registry address: %w", err)
	}

	tpl, err := template.ParseFiles(fpath)
	if err != nil {
		return nil, fmt.Errorf("error parsing tag template: %w", err)
	}

	tpvals := map[string]string{
		"Namespace": ns,
		"Name":      name,
		"Registry":  inregaddr,
	}
	buf := bytes.NewBuffer(nil)
	if err := tpl.Execute(buf, tpvals); err != nil {
		return nil, fmt.Errorf("error executing tag template: %w", err)
	}

	it := &imagtagv1.Tag{}
	if err := json.NewDecoder(buf).Decode(it); err != nil {
		return nil, fmt.Errorf("error decoding tag json: %w", err)
	}
	return it, nil
}

// cleanAndEncodeTag json encodes the Tag referred by TagExport and stores it in
// a file callled tag.json inside the provided directory.
func (t *TagIO) cleanAndEncodeTag(it *imagtagv1.Tag, dir string) error {
	tcopy, err := t.cleanTag(it)
	if err != nil {
		return fmt.Errorf("unable to clear tag: %w", err)
	}

	tfpath := fmt.Sprintf("%s/tag.json", dir)
	tf, err := os.Create(tfpath)
	if err != nil {
		return fmt.Errorf("error creating file for encoded tag: %w", err)
	}
	defer tf.Close()

	return json.NewEncoder(tf).Encode(tcopy)
}

// cleanTag changes all references to the cache registry by a template entry so it
// can be reassembled later on in another cluster. Cleans up the tag namespace and
// name as well, replacing them by template entries as well.
func (t *TagIO) cleanTag(it *imagtagv1.Tag) (*imagtagv1.Tag, error) {
	inregaddr, _, err := t.syssvc.CacheRegistryAddresses()
	if err != nil {
		return nil, fmt.Errorf("error cleaning up tag: %w", err)
	}

	it = it.DeepCopy()
	namespace := fmt.Sprintf("/%s/", it.Namespace)
	it.ObjectMeta = metav1.ObjectMeta{
		Name:      "{{.Name}}",
		Namespace: "{{.Namespace}}",
	}
	for i := range it.Status.References {
		imgref := it.Status.References[i].ImageReference
		if !strings.HasPrefix(imgref, inregaddr) {
			continue
		}

		imgref = strings.ReplaceAll(imgref, inregaddr, "{{.Registry}}")
		imgref = strings.ReplaceAll(imgref, namespace, "/{{.Namespace}}/")
		it.Status.References[i].ImageReference = imgref
	}
	return it, nil
}

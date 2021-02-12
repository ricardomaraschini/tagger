package fs

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestTempDir(t *testing.T) {
	fs := &FS{}
	dir, clean, err := fs.TempDir()
	if err != nil {
		t.Fatalf("unexpected error creating dir: %s", err)
	}

	dh, err := os.Open(dir)
	if err != nil {
		t.Fatalf("unexpected error opening dir: %s", err)
	}
	defer dh.Close()

	di, err := dh.Stat()
	if err != nil {
		t.Fatalf("unexpected error stat dir: %s", err)
	}

	if !di.IsDir() {
		t.Error("TempDir did not return a directory")
	}

	clean()
	if _, err = os.Open(dir); err == nil {
		t.Error("temporary dir not deleted")
	}

	if _, ok := err.(*os.PathError); !ok {
		t.Errorf("returned error is not of type path error: %s", err)
	}
}

func TestTempFile(t *testing.T) {
	fs := &FS{}
	fp, clean, err := fs.TempFile()
	if err != nil {
		t.Fatalf("unexpected error creating file: %s", err)
	}

	fi, err := fp.Stat()
	if err != nil {
		t.Fatalf("unexpected error stat file: %s", err)
	}

	if fi.IsDir() {
		t.Error("TempFile returned a directory")
	}

	clean()
	if _, err = ioutil.ReadAll(fp); err == nil {
		t.Errorf("temp file not closed")
	}

	if !strings.Contains(err.Error(), "file already closed") {
		t.Errorf("unexpected error message reading file: %s", err)
	}

	if _, err = os.Open(fp.Name()); err == nil {
		t.Error("temporary file not deleted")
	}

	if _, ok := err.(*os.PathError); !ok {
		t.Errorf("returned error is not of type path error: %s", err)
	}
}

func TestArchiveDirectory(t *testing.T) {
	fs := &FS{}

	dir, cleandir, err := fs.TempDir()
	if err != nil {
		t.Fatalf("unexpected error creating dir: %s", err)
	}
	defer cleandir()

	for i := 0; i < 100; i++ {
		fpath := fmt.Sprintf("%s/%d.txt", dir, i)
		fp, err := os.Create(fpath)
		if err != nil {
			t.Fatalf("unexpected error creating file for tar: %s", err)
		}

		if _, err := fp.Write([]byte("testing")); err != nil {
			t.Fatalf("unexpected error writing to file: %s", err)
		}
		fp.Close()
	}

	tar, cleanfile, err := fs.TempFile()
	if err != nil {
		t.Fatalf("unexpected error creating temp tar file: %s", err)
	}
	defer cleanfile()

	if err := fs.ArchiveDirectory(dir, tar); err != nil {
		t.Fatalf("unexpected error archiving dir: %s", err)
	}

	tar.Seek(0, 0)

	buffer := make([]byte, 512)
	if _, err = tar.Read(buffer); err != nil {
		t.Fatalf("error reading file header: %s", err)
	}

	ctype := http.DetectContentType(buffer)
	if ctype != "application/octet-stream" {
		t.Fatalf("unexpected file type %q", ctype)
	}
}

func TestUnarchiveFile(t *testing.T) {
	fs := &FS{}

	dir, cleandir, err := fs.TempDir()
	if err != nil {
		t.Fatalf("unexpected error creating dir: %s", err)
	}
	defer cleandir()

	// create 10 sub directories containing 10 files each.
	for i := 0; i < 10; i++ {
		subdir := fmt.Sprintf("%s/%d", dir, i)
		if err := os.Mkdir(subdir, 0755); err != nil {
			t.Fatalf("error creating temp dir for test: %s", err)
		}
		for j := 0; j < 10; j++ {
			fpath := fmt.Sprintf("%s/%d.txt", subdir, j)

			fp, err := os.Create(fpath)
			if err != nil {
				t.Fatalf("unexpected error creating file for tar: %s", err)
			}

			if _, err := fp.Write([]byte("testing")); err != nil {
				fp.Close()
				t.Fatalf("unexpected error writing to file: %s", err)
			}

			fp.Close()
		}
	}

	// create a temp file and archive a temp directory into it.
	tar, cleanfile, err := fs.TempFile()
	if err != nil {
		t.Fatalf("unexpected error creating temp tar file: %s", err)
	}
	defer cleanfile()

	if err := fs.ArchiveDirectory(dir, tar); err != nil {
		t.Fatalf("unexpected error archiving dir: %s", err)
	}
	tar.Seek(0, 0)

	// create another dir to unarchive the archived file
	dir, cleandir, err = fs.TempDir()
	if err != nil {
		t.Fatalf("error creating temp dir: %s", err)
	}
	defer cleandir()

	if err := fs.UnarchiveFile(tar, dir); err != nil {
		t.Fatalf("unexpected error unarchive tar: %s", err)
	}

	// now checks the output of the unarchive process. we must find
	// 10 sub directories with 10 files each. Checks also each file
	// content.
	for i := 0; i < 10; i++ {
		subdir := fmt.Sprintf("%s/%d", dir, i)
		dh, err := os.Open(subdir)
		if err != nil {
			dh.Close()
			t.Fatalf("error opening unarchive dir: %s", err)
		}
		dh.Close()

		for j := 0; j < 10; j++ {
			fpath := fmt.Sprintf("%s/%d.txt", subdir, j)
			fp, err := os.Open(fpath)
			if err != nil {
				t.Fatalf("unexpected opening unarchived file: %s", err)
			}

			dt, err := ioutil.ReadAll(fp)
			if err != nil {
				fp.Close()
				t.Fatalf("unexpected error reading file: %s", err)
			}

			if string(dt) != "testing" {
				fp.Close()
				t.Fatalf("invalid content for file: %s", string(dt))
			}

			fp.Close()
		}
	}
}

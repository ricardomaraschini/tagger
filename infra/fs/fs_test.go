// Copyright 2020 The Tagger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fs

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func TestOptions(t *testing.T) {
	f := New(WithTmpDir("/abc"))
	if f.tmpdir != "/abc" {
		t.Errorf("temp dir not being set by option")
	}
}

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

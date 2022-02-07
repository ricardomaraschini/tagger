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

	"k8s.io/klog/v2"
)

// Option sets an option in a FS instance.
type Option func(*FS)

// WithTmpDir sets a different base temp directory.
func WithTmpDir(tmpdir string) Option {
	return func(f *FS) {
		f.tmpdir = tmpdir
	}
}

// FS gathers services related to filesystem operations.
type FS struct {
	tmpdir string
}

// New returns a handler for filesystem related activities.
func New(opts ...Option) *FS {
	f := &FS{}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// TempDir creates and returns a temporary dir inside our base temp dir specified on FS.tmpdir.
// Returns the directory path, a clean up function (delete dir) or an error.
func (f *FS) TempDir() (string, func(), error) {
	dir, err := ioutil.TempDir(f.tmpdir, "tmp-dir-*")
	if err != nil {
		return "", nil, err
	}

	clean := func() {
		if err := os.RemoveAll(dir); err != nil {
			klog.Errorf("error removing temp directory: %s", err)
		}
	}
	return dir, clean, nil
}

// TempFile creates and returns a temporary file inside our base temp directory.  Returns the
// opened file, a clean up function (close and delete file) or an error.
func (f *FS) TempFile() (*os.File, func(), error) {
	fp, err := ioutil.TempFile(f.tmpdir, "tmp-file-*")
	if err != nil {
		return nil, nil, err
	}

	clean := func() {
		if err := fp.Close(); err != nil {
			klog.Errorf("error closing temp file: %s", err)
		}
		if err := os.Remove(fp.Name()); err != nil {
			klog.Errorf("error removing temp directory: %s", err)
		}
	}
	return fp, clean, nil
}

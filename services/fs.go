package services

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
)

// FS gathers services related to filesystem operations.
type FS struct{}

// NewFS returns a handler for filesystem related activities.
func NewFS() *FS {
	return &FS{}
}

// CompressDirectory creates a tarball file from directory pointed by
// srcdir, writing the output into dst.
func (f *FS) CompressDirectory(srcdir string, dst io.Writer) error {
	zw := gzip.NewWriter(dst)
	defer zw.Close()
	tw := tar.NewWriter(zw)
	defer tw.Close()

	return filepath.Walk(
		srcdir,
		func(file string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			header, err := tar.FileInfoHeader(fi, file)
			if err != nil {
				return err
			}

			relpath, err := filepath.Rel(srcdir, file)
			if err != nil {
				return err
			}

			header.Name = relpath
			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			if fi.IsDir() {
				return nil
			}

			data, err := os.Open(file)
			if err != nil {
				return err
			}

			_, err = io.Copy(tw, data)
			return err
		},
	)
}

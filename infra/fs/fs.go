package fs

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"k8s.io/klog/v2"
)

// FS gathers services related to filesystem operations.
type FS struct {
	tmpdir string
}

// New returns a handler for filesystem related activities.
func New(tmpdir string) *FS {
	return &FS{
		tmpdir: tmpdir,
	}
}

// TempDir creates and returns a temporary dir inside our base temp dir
// specified on FS.tmpdir. Returns the directory path, a clean up function
// (delete dir) or an error.
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

// TempFile creates and returns a temporary file inside our base temp directory.
// Returns the opened file, a clean up function (close and delete file) or an
// error.
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

// ArchiveDirectory creates a tar archive file from directory pointed
// by srcdir, writing the output into dst.
func (f *FS) ArchiveDirectory(srcdir string, dst io.Writer) error {
	tw := tar.NewWriter(dst)
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

			if header.Name == "." {
				return nil
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
			defer data.Close()

			_, err = io.Copy(tw, data)
			return err
		},
	)
}

// UnarchiveFile unarchives a tar file pointed by src storing results inside
// dst directory.
func (f *FS) UnarchiveFile(src io.Reader, dst string) error {
	tarfp := tar.NewReader(src)
	for {
		hdr, err := tarfp.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("error extracting tar: %w", err)
		}

		if hdr.Name == "." {
			continue
		}

		path := fmt.Sprintf("%s/%s", dst, hdr.Name)

		// from this point on we expect the type to be or regular file
		// or a regular directory, if not any of these we log an error
		// and move forward.
		if hdr.Typeflag == tar.TypeDir {
			if err := os.Mkdir(path, 0755); err != nil {
				return fmt.Errorf("error creating dir: %w", err)
			}
			continue
		}

		if hdr.Typeflag != tar.TypeReg {
			klog.Errorf("ignoring unknown file: %s", hdr.Name)
			continue
		}

		out, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("error creating file: %w", err)
		}
		if _, err := io.Copy(out, tarfp); err != nil {
			out.Close()
			return fmt.Errorf("error decompressing: %w", err)
		}
		out.Close()
	}
	return nil
}

// MoveFiles move regular files from one directory into another. This is
// not a recursive function (it does not copy subdirectories).
func (f *FS) MoveFiles(srcdir, dstdir string) error {
	return filepath.Walk(
		srcdir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			dstfile := fmt.Sprintf("%s/%s", dstdir, info.Name())
			return os.Rename(path, dstfile)
		},
	)
}

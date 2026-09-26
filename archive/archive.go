package archive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// ErrUnsupportedEntry is returned when Extract encounters a tar entry type it
// cannot restore (e.g. devices or fifos).
var ErrUnsupportedEntry = errors.New("unsupported archive entry type")

// hardLinkKey identifies a file by device and inode for hard-link detection.
type hardLinkKey struct {
	dev uint64
	ino uint64
}

// Create serializes the directory rooted at src into a compressed tar stream
// written to w. Entries are stored relative to src and retain their mode and
// modification time. Symlinks are preserved verbatim: the link target is
// recorded without being resolved or followed.
func Create(ctx context.Context, src string, w io.Writer, compression Compression) error {
	comp, err := compression.newWriter(w)
	if err != nil {
		return err
	}

	tw := tar.NewWriter(comp)

	root, err := os.OpenRoot(src)
	if err != nil {
		return err
	}

	defer root.Close()

	hardLinks := make(map[hardLinkKey]string)

	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		if rel == "." {
			return nil
		}

		return writeEntry(root, tw, rel, d, hardLinks)
	})
	if err != nil {
		_ = tw.Close()
		_ = comp.Close()

		return err
	}

	if err := tw.Close(); err != nil {
		return err
	}

	return comp.Close()
}

// writeEntry serializes a single filesystem entry into tw. File content and
// symlink targets are read through root so they are confined to src. Regular
// files with more than one hard link are emitted once with their content and
// as a TypeLink header on every subsequent occurrence.
func writeEntry(root *os.Root, tw *tar.Writer, rel string, d fs.DirEntry, hardLinks map[hardLinkKey]string) error {
	info, err := d.Info()
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}

	header.Name = filepath.ToSlash(rel)

	switch {
	case info.Mode()&os.ModeSymlink != 0:
		link, err := root.Readlink(rel)
		if err != nil {
			return err
		}

		header.Typeflag = tar.TypeSymlink
		header.Linkname = link
		header.Size = 0
	case info.Mode().IsRegular():
		if key, ok := hardLinkKeyFor(info); ok {
			if first, exists := hardLinks[key]; exists {
				header.Typeflag = tar.TypeLink
				header.Linkname = first
				header.Size = 0
			} else {
				hardLinks[key] = header.Name
			}
		}
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	if header.Typeflag != tar.TypeReg {
		return nil
	}

	f, err := root.Open(rel)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(tw, f)
	closeErr := f.Close()

	if copyErr != nil {
		return copyErr
	}

	return closeErr
}

// hardLinkKeyFor returns the device/inode key for a regular file that is part
// of a hard-link group, or ok=false when the platform does not report it or
// the file has no other links.
func hardLinkKeyFor(info fs.FileInfo) (hardLinkKey, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink <= 1 {
		return hardLinkKey{}, false
	}

	//nolint:unconvert // Dev/Ino are not uint64 on every platform; the conversions keep the package portable.
	return hardLinkKey{dev: uint64(stat.Dev), ino: uint64(stat.Ino)}, true
}

// Extract restores a compressed tar stream read from r into the directory
// dest. All paths are confined to dest: entries with absolute or parent
// traversal names are rejected, and writes are resolved against dest through
// os.Root so a symlink planted earlier in the stream cannot redirect a write
// outside dest.
func Extract(ctx context.Context, dest string, r io.Reader, compression Compression) error {
	decomp, err := compression.newReader(r)
	if err != nil {
		return err
	}

	defer decomp.Close()

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	root, err := os.OpenRoot(dest)
	if err != nil {
		return fmt.Errorf("open destination root: %w", err)
	}

	defer root.Close()

	tr := tar.NewReader(decomp)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return err
		}

		if err := extractEntry(root, tr, header); err != nil {
			return err
		}
	}
}

// extractEntry restores a single tar entry within root.
func extractEntry(root *os.Root, tr *tar.Reader, header *tar.Header) error {
	name := filepath.FromSlash(header.Name)

	switch header.Typeflag {
	case tar.TypeDir:
		if err := root.MkdirAll(name, header.FileInfo().Mode().Perm()); err != nil {
			return fmt.Errorf("create directory %q: %w", header.Name, err)
		}
	case tar.TypeReg:
		if err := ensureParent(root, name); err != nil {
			return err
		}

		if err := writeFile(root, name, tr, header.FileInfo().Mode().Perm()); err != nil {
			return err
		}
	case tar.TypeSymlink:
		if err := ensureParent(root, name); err != nil {
			return err
		}

		// Replace any existing entry so a previously extracted file or
		// symlink at the same path cannot be reused.
		_ = root.Remove(name)

		if err := root.Symlink(header.Linkname, name); err != nil {
			return fmt.Errorf("create symlink %q: %w", header.Name, err)
		}

		return nil
	case tar.TypeLink:
		if err := ensureParent(root, name); err != nil {
			return err
		}

		_ = root.Remove(name)

		target := filepath.FromSlash(header.Linkname)
		if err := root.Link(target, name); err != nil {
			return fmt.Errorf("create hard link %q -> %q: %w", header.Name, header.Linkname, err)
		}

		return nil
	default:
		return fmt.Errorf("%w: %q (type %d)", ErrUnsupportedEntry, header.Name, header.Typeflag)
	}

	if !header.ModTime.IsZero() {
		if err := root.Chtimes(name, header.ModTime, header.ModTime); err != nil {
			return fmt.Errorf("restore modification time for %q: %w", header.Name, err)
		}
	}

	return nil
}

// ensureParent creates the parent directory of name when it is not the root.
func ensureParent(root *os.Root, name string) error {
	parent := filepath.Dir(name)
	if parent == "." {
		return nil
	}

	if err := root.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create directory %q: %w", parent, err)
	}

	return nil
}

// writeFile writes the next regular file from tr to name within root.
func writeFile(root *os.Root, name string, r io.Reader, mode os.FileMode) error {
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create file %q: %w", name, err)
	}

	_, copyErr := io.Copy(f, r)
	closeErr := f.Close()

	if copyErr != nil {
		return fmt.Errorf("write file %q: %w", name, copyErr)
	}

	if closeErr != nil {
		return fmt.Errorf("close file %q: %w", name, closeErr)
	}

	return nil
}

// newWriter wraps w with the configured compressor. The returned WriteCloser
// must be closed to flush the stream, but closing it does not close w.
func (c Compression) newWriter(w io.Writer) (io.WriteCloser, error) {
	switch c {
	case CompressionGzip:
		return gzip.NewWriter(w), nil
	case CompressionNone:
		return nopWriteCloser{w}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidCompression, c)
	}
}

// newReader wraps r with the configured decompressor.
func (c Compression) newReader(r io.Reader) (io.ReadCloser, error) {
	switch c {
	case CompressionGzip:
		return gzip.NewReader(r)
	case CompressionNone:
		return io.NopCloser(r), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidCompression, c)
	}
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error {
	return nil
}

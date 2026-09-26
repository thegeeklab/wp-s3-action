package archive

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tarEntry struct {
	name     string
	typeflag byte
	mode     int64
	content  string
	linkname string
}

func buildTar(t *testing.T, entries ...tarEntry) *bytes.Reader {
	t.Helper()

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)

	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			switch e.typeflag {
			case tar.TypeDir:
				mode = 0o755
			case tar.TypeSymlink:
				mode = 0o777
			default:
				mode = 0o644
			}
		}

		header := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Mode:     mode,
			Size:     int64(len(e.content)),
			Linkname: e.linkname,
		}

		require.NoError(t, tw.WriteHeader(header))

		if e.typeflag == tar.TypeReg && e.content != "" {
			_, err := tw.Write([]byte(e.content))
			require.NoError(t, err)
		}
	}

	require.NoError(t, tw.Close())

	return bytes.NewReader(buf.Bytes())
}

func assertFile(t *testing.T, root, name, content string, mode os.FileMode) {
	t.Helper()

	full := filepath.Join(root, name)

	got, err := os.ReadFile(full)
	require.NoError(t, err)
	assert.Equal(t, content, string(got))

	info, err := os.Lstat(full)
	require.NoError(t, err)
	assert.Equal(t, mode, info.Mode().Perm())
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.NoError(t, os.Chmod(path, mode))
}

func TestCreateExtractRoundTrip(t *testing.T) {
	t.Parallel()

	for _, compression := range []Compression{CompressionGzip, CompressionNone} {
		t.Run(compression.String(), func(t *testing.T) {
			t.Parallel()

			src := t.TempDir()

			require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(src, "bin"), 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(src, "empty"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o600))
			writeTestFile(t, filepath.Join(src, "sub", "b.txt"), "world", 0o644)
			writeTestFile(t, filepath.Join(src, "bin", "tool"), "#!/bin/sh\n", 0o755)
			require.NoError(t, os.Symlink("a.txt", filepath.Join(src, "sub", "link")))

			var buf bytes.Buffer

			require.NoError(t, Create(t.Context(), src, &buf, compression))

			dest := t.TempDir()

			require.NoError(t, Extract(t.Context(), dest, &buf, compression))

			assertFile(t, dest, "a.txt", "hello", 0o600)
			assertFile(t, dest, "sub/b.txt", "world", 0o644)
			assertFile(t, dest, "bin/tool", "#!/bin/sh\n", 0o755)

			info, err := os.Stat(filepath.Join(dest, "empty"))
			require.NoError(t, err)
			assert.True(t, info.IsDir())

			link, err := os.Readlink(filepath.Join(dest, "sub", "link"))
			require.NoError(t, err)
			assert.Equal(t, "a.txt", link)
		})
	}
}

func TestExtractRejectsPathTraversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		entries []tarEntry
	}{
		{
			name:    "relative traversal",
			entries: []tarEntry{{name: "../evil.txt", typeflag: tar.TypeReg, content: "x"}},
		},
		{
			name:    "absolute path",
			entries: []tarEntry{{name: "/evil.txt", typeflag: tar.TypeReg, content: "x"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dest := t.TempDir()

			err := Extract(t.Context(), dest, buildTar(t, tt.entries...), CompressionNone)
			require.Error(t, err)

			escaped := filepath.Join(filepath.Dir(dest), "evil.txt")
			_, statErr := os.Stat(escaped)
			assert.True(t, os.IsNotExist(statErr), "file must not escape the destination root")
		})
	}
}

func TestExtractRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	dest := t.TempDir()
	outside := t.TempDir()

	entries := []tarEntry{
		{name: "link", typeflag: tar.TypeSymlink, linkname: outside},
		{name: "link/evil.txt", typeflag: tar.TypeReg, content: "pwned"},
	}

	err := Extract(t.Context(), dest, buildTar(t, entries...), CompressionNone)
	require.Error(t, err)

	_, statErr := os.Stat(filepath.Join(outside, "evil.txt"))
	assert.True(t, os.IsNotExist(statErr), "file must not be written through a symlink that escapes the root")
}

func TestExtractUnsupportedType(t *testing.T) {
	t.Parallel()

	entries := []tarEntry{{name: "fifo", typeflag: tar.TypeFifo}}

	err := Extract(t.Context(), t.TempDir(), buildTar(t, entries...), CompressionNone)
	assert.Error(t, err)
}

func TestCreateExtractHardLinks(t *testing.T) {
	t.Parallel()

	for _, compression := range []Compression{CompressionGzip, CompressionNone} {
		t.Run(compression.String(), func(t *testing.T) {
			t.Parallel()

			src := t.TempDir()

			writeTestFile(t, filepath.Join(src, "a.txt"), "shared", 0o644)
			require.NoError(t, os.Link(filepath.Join(src, "a.txt"), filepath.Join(src, "b.txt")))

			var buf bytes.Buffer

			require.NoError(t, Create(t.Context(), src, &buf, compression))

			dest := t.TempDir()

			require.NoError(t, Extract(t.Context(), dest, &buf, compression))

			assertFile(t, dest, "a.txt", "shared", 0o644)
			assertFile(t, dest, "b.txt", "shared", 0o644)

			a, err := os.Stat(filepath.Join(dest, "a.txt"))
			require.NoError(t, err)

			b, err := os.Stat(filepath.Join(dest, "b.txt"))
			require.NoError(t, err)

			assert.True(t, os.SameFile(a, b), "hard link must be preserved across the round trip")
		})
	}
}

func TestExtractHardLink(t *testing.T) {
	t.Parallel()

	entries := []tarEntry{
		{name: "a.txt", typeflag: tar.TypeReg, content: "shared", mode: 0o644},
		{name: "b.txt", typeflag: tar.TypeLink, linkname: "a.txt", mode: 0o644},
	}

	dest := t.TempDir()

	require.NoError(t, Extract(t.Context(), dest, buildTar(t, entries...), CompressionNone))

	assertFile(t, dest, "a.txt", "shared", 0o644)

	a, err := os.Stat(filepath.Join(dest, "a.txt"))
	require.NoError(t, err)

	b, err := os.Stat(filepath.Join(dest, "b.txt"))
	require.NoError(t, err)

	assert.True(t, os.SameFile(a, b), "extracted hard link must reference the same inode")
}

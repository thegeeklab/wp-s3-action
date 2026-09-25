package plugin

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thegeeklab/wp-s3-action/aws"
	"github.com/urfave/cli/v3"
)

func setupPluginTest(t *testing.T) (*Plugin, error) {
	t.Helper()

	cli.HelpPrinter = func(_ io.Writer, _ string, _ interface{}) {}
	got := New(func(_ context.Context) error { return nil })
	err := got.App.Run(t.Context(), []string{"wp-s3-action"})

	return got, err
}

func TestACLFlag(t *testing.T) {
	tests := []struct {
		name string
		envs map[string]string
		want map[string]string
	}{
		{
			name: "empty ACL",
			envs: map[string]string{},
			want: map[string]string{},
		},
		{
			name: "single ACL entry",
			envs: map[string]string{
				"PLUGIN_UPLOAD_ACL": `{"public/*":"public-read"}`,
			},
			want: map[string]string{
				"public/*": "public-read",
			},
		},
		{
			name: "multiple ACL entries",
			envs: map[string]string{
				"PLUGIN_UPLOAD_ACL": `{"public/*":"public-read","private/*":"private"}`,
			},
			want: map[string]string{
				"public/*":  "public-read",
				"private/*": "private",
			},
		},
		{
			name: "ACL with special characters",
			envs: map[string]string{
				"PLUGIN_UPLOAD_ACL": `{"*.jpg":"public-read","*.pdf":"authenticated-read"}`,
			},
			want: map[string]string{
				"*.jpg": "public-read",
				"*.pdf": "authenticated-read",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envs {
				t.Setenv(key, value)
			}

			got, _ := setupPluginTest(t)

			assert.EqualValues(t, tt.want, got.Settings.Upload.ACL)
		})
	}
}

func TestMetadataFlag(t *testing.T) {
	tests := []struct {
		name string
		envs map[string]string
		want map[string]map[string]string
	}{
		{
			name: "empty metadata",
			envs: map[string]string{},
			want: map[string]map[string]string{},
		},
		{
			name: "single metadata entry",
			envs: map[string]string{
				"PLUGIN_UPLOAD_METADATA": `{"*.html":{"Cache-Control":"max-age=3600"}}`,
			},
			want: map[string]map[string]string{
				"*.html": {
					"Cache-Control": "max-age=3600",
				},
			},
		},
		{
			name: "multiple metadata entries for single pattern",
			envs: map[string]string{
				"PLUGIN_UPLOAD_METADATA": `{"*.html":{"Cache-Control":"max-age=3600","Content-Type":"text/html"}}`,
			},
			want: map[string]map[string]string{
				"*.html": {
					"Cache-Control": "max-age=3600",
					"Content-Type":  "text/html",
				},
			},
		},
		{
			name: "multiple patterns with metadata",
			envs: map[string]string{
				"PLUGIN_UPLOAD_METADATA": `{
					"*.html":{"Cache-Control":"max-age=3600","Content-Type":"text/html"},
					"*.jpg":{"Cache-Control":"max-age=86400","Content-Type":"image/jpeg"}
				}`,
			},
			want: map[string]map[string]string{
				"*.html": {
					"Cache-Control": "max-age=3600",
					"Content-Type":  "text/html",
				},
				"*.jpg": {
					"Cache-Control": "max-age=86400",
					"Content-Type":  "image/jpeg",
				},
			},
		},
		{
			name: "complex metadata with special characters",
			envs: map[string]string{
				"PLUGIN_UPLOAD_METADATA": `{
					"*.pdf":{"Content-Disposition":"attachment; filename=\"document.pdf\"","x-amz-meta-owner":"John Doe"},
					"data/*.json":{"x-amz-meta-created":"2023-01-01","x-amz-meta-version":"1.0.0"}
				}`,
			},
			want: map[string]map[string]string{
				"*.pdf": {
					"Content-Disposition": "attachment; filename=\"document.pdf\"",
					"x-amz-meta-owner":    "John Doe",
				},
				"data/*.json": {
					"x-amz-meta-created": "2023-01-01",
					"x-amz-meta-version": "1.0.0",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envs {
				t.Setenv(key, value)
			}

			got, _ := setupPluginTest(t)

			assert.EqualValues(t, tt.want, got.Settings.Upload.Metadata)
		})
	}
}

func TestRedirectsFlag(t *testing.T) {
	tests := []struct {
		name string
		envs map[string]string
		want map[string]string
	}{
		{
			name: "empty redirects",
			envs: map[string]string{},
			want: map[string]string{},
		},
		{
			name: "single redirect entry as JSON",
			envs: map[string]string{
				"PLUGIN_REDIRECTS": `{"old/path":"https://example.com/new/path"}`,
			},
			want: map[string]string{
				"old/path": "https://example.com/new/path",
			},
		},
		{
			name: "multiple redirect entries",
			envs: map[string]string{
				"PLUGIN_REDIRECTS": `{
					"old/path1":"https://example.com/new/path1",
					"old/path2":"https://example.com/new/path2"
				}`,
			},
			want: map[string]string{
				"old/path1": "https://example.com/new/path1",
				"old/path2": "https://example.com/new/path2",
			},
		},
		{
			name: "fallback to '*' for non-map string",
			envs: map[string]string{
				"PLUGIN_REDIRECTS": "https://example.com/fallback",
			},
			want: map[string]string{
				"*": "https://example.com/fallback",
			},
		},
		{
			name: "special characters in paths",
			envs: map[string]string{
				"PLUGIN_REDIRECTS": `{
					"path/with/special?chars":"https://example.com/new?param=value",
					"another/path#fragment":"https://example.com/another#section"
				}`,
			},
			want: map[string]string{
				"path/with/special?chars": "https://example.com/new?param=value",
				"another/path#fragment":   "https://example.com/another#section",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envs {
				t.Setenv(key, value)
			}

			got, _ := setupPluginTest(t)

			assert.EqualValues(t, tt.want, got.Settings.Redirects)
		})
	}
}

func TestChecksumCalculationFlag(t *testing.T) {
	tests := []struct {
		name    string
		envs    map[string]string
		want    string
		wantErr error
	}{
		{
			name: "default value",
			envs: map[string]string{},
			want: string(aws.ChecksumRequired),
		},
		{
			name: "set to supported",
			envs: map[string]string{
				"PLUGIN_CHECKSUM_CALCULATION": "supported",
			},
			want: string(aws.ChecksumSupported),
		},
		{
			name: "set to required",
			envs: map[string]string{
				"PLUGIN_CHECKSUM_CALCULATION": "required",
			},
			want: string(aws.ChecksumRequired),
		},
		{
			name: "invalid value causes error",
			envs: map[string]string{
				"PLUGIN_CHECKSUM_CALCULATION": "invalid",
			},
			wantErr: aws.ErrInvalidChecksumCalculationMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envs {
				t.Setenv(key, value)
			}

			got, err := setupPluginTest(t)

			if tt.wantErr != nil {
				assert.ErrorAs(t, err, &tt.wantErr)

				return
			}

			assert.Equal(t, tt.want, got.Settings.ChecksumCalculation)
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		settings *Settings
		wantErr  error
	}{
		{
			name:     "upload action is valid",
			settings: &Settings{ActionStrings: []string{"upload"}, MaxConcurrency: 1},
		},
		{
			name:     "download action without target",
			settings: &Settings{ActionStrings: []string{"download"}, MaxConcurrency: 1},
			wantErr:  ErrDownloadTarget,
		},
		{
			name:     "download action with target",
			settings: &Settings{ActionStrings: []string{"download"}, Target: "prefix/", MaxConcurrency: 1},
		},
		{
			name:     "download action rejects bucket-root prefix",
			settings: &Settings{ActionStrings: []string{"download"}, Target: "/", MaxConcurrency: 1},
			wantErr:  ErrDownloadTarget,
		},
		{
			name:     "delete action without target",
			settings: &Settings{ActionStrings: []string{"delete"}, MaxConcurrency: 1},
			wantErr:  ErrTargetNotSet,
		},
		{
			name:     "delete action with target",
			settings: &Settings{ActionStrings: []string{"delete"}, Target: "prefix/", MaxConcurrency: 1},
		},
		{
			name:     "redirect action without redirects",
			settings: &Settings{ActionStrings: []string{"redirect"}, MaxConcurrency: 1},
			wantErr:  ErrRedirectsNotSet,
		},
		{
			name:     "invalidate-cloudfront action without distribution",
			settings: &Settings{ActionStrings: []string{"invalidate-cloudfront"}, MaxConcurrency: 1},
			wantErr:  ErrCloudFrontDistribution,
		},
		{
			name:     "unknown action",
			settings: &Settings{ActionStrings: []string{"bogus"}, MaxConcurrency: 1},
			wantErr:  ErrActionUnknown,
		},
		{
			name:     "max-concurrency must be at least 1",
			settings: &Settings{ActionStrings: []string{"upload"}, MaxConcurrency: 0},
			wantErr:  ErrInvalidMaxConcurrency,
		},
		{
			name:     "negative max-concurrency is rejected",
			settings: &Settings{ActionStrings: []string{"upload"}, MaxConcurrency: -1},
			wantErr:  ErrInvalidMaxConcurrency,
		},
		{
			name:     "valid max-concurrency",
			settings: &Settings{ActionStrings: []string{"upload"}, MaxConcurrency: 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &Plugin{Settings: tt.settings}

			err := p.Validate()

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestRemoteKeyToLocalPath(t *testing.T) {
	t.Parallel()

	source := t.TempDir()

	tests := []struct {
		name      string
		remoteKey string
		target    string
		source    string
		want      string
		wantErr   error
	}{
		{
			name:      "strip target prefix",
			remoteKey: "foo/bar.txt",
			target:    "foo",
			source:    source,
			want:      filepath.Join(source, "bar.txt"),
		},
		{
			name:      "keep sibling key sharing target string prefix",
			remoteKey: "foobar/bar.txt",
			target:    "foo",
			source:    source,
			want:      filepath.Join(source, "foobar", "bar.txt"),
		},
		{
			name:      "empty target",
			remoteKey: "bar.txt",
			target:    "",
			source:    source,
			want:      filepath.Join(source, "bar.txt"),
		},
		{
			name:      "target with trailing slash",
			remoteKey: "foo/bar.txt",
			target:    "foo/",
			source:    source,
			want:      filepath.Join(source, "bar.txt"),
		},
		{
			name:      "reject path traversal",
			remoteKey: "../etc/passwd",
			target:    "",
			source:    source,
			wantErr:   ErrPathTraversal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := remoteKeyToLocalPath(tt.remoteKey, tt.target, tt.source)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRemoteKeyToLocalPathSymlink(t *testing.T) {
	t.Parallel()

	source := t.TempDir()
	outside := t.TempDir()

	require.NoError(t, os.Symlink(outside, filepath.Join(source, "link")))

	_, err := remoteKeyToLocalPath("link/passwd", "", source)
	assert.ErrorIs(t, err, ErrPathTraversal)
}

func TestPathWithinRoot(t *testing.T) {
	t.Parallel()

	missingRoot := filepath.Join(t.TempDir(), "does-not-exist")
	containingRoot := t.TempDir()

	// plant a symlinked intermediate that points outside the root
	sibling := t.TempDir()
	require.NoError(t, os.Symlink(sibling, filepath.Join(containingRoot, "escape")))

	tests := []struct {
		name string
		root string
		path string
		want bool
	}{
		{
			name: "missing root is not contained",
			root: missingRoot,
			path: "/etc/passwd",
			want: false,
		},
		{
			name: "existing root contains its descendant",
			root: containingRoot,
			path: filepath.Join(containingRoot, "sub", "file.txt"),
			want: true,
		},
		{
			name: "path outside root is not contained",
			root: containingRoot,
			path: filepath.Join(missingRoot, "file.txt"),
			want: false,
		},
		{
			name: "non-existent leaf under root is allowed (to be created)",
			root: containingRoot,
			path: filepath.Join(containingRoot, "newdir", "file.txt"),
			want: true,
		},
		{
			name: "leaf resolves through symlinked intermediate to escape root",
			root: containingRoot,
			path: filepath.Join(containingRoot, "escape", "file.txt"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, pathWithinRoot(tt.root, tt.path))
		})
	}
}

func TestRunJobsUnknownAction(t *testing.T) {
	t.Parallel()

	p := &Plugin{
		Settings: &Settings{
			Bucket:         "test-bucket",
			MaxConcurrency: 1,
		},
	}

	jobs := make(chan Job, 1)
	jobs <- Job{local: "x", remote: "y", action: S3Action("bogus")}

	close(jobs)

	err := p.runJobs(t.Context(), nil, S3Action("bogus"), jobs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

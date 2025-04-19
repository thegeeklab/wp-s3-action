package plugin

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
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
				"PLUGIN_ACL": `{"public/*":"public-read"}`,
			},
			want: map[string]string{
				"public/*": "public-read",
			},
		},
		{
			name: "multiple ACL entries",
			envs: map[string]string{
				"PLUGIN_ACL": `{"public/*":"public-read","private/*":"private"}`,
			},
			want: map[string]string{
				"public/*":  "public-read",
				"private/*": "private",
			},
		},
		{
			name: "ACL with special characters",
			envs: map[string]string{
				"PLUGIN_ACL": `{"*.jpg":"public-read","*.pdf":"authenticated-read"}`,
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

			assert.EqualValues(t, tt.want, got.Settings.ACL)
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
				"PLUGIN_METADATA": `{"*.html":{"Cache-Control":"max-age=3600"}}`,
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
				"PLUGIN_METADATA": `{"*.html":{"Cache-Control":"max-age=3600","Content-Type":"text/html"}}`,
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
				"PLUGIN_METADATA": `{
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
				"PLUGIN_METADATA": `{
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

			assert.EqualValues(t, tt.want, got.Settings.Metadata)
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

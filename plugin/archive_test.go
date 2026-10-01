package plugin

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	plugin_base "github.com/thegeeklab/wp-plugin-go/v8/plugin"
	"github.com/thegeeklab/wp-s3-action/archive"
	"github.com/thegeeklab/wp-s3-action/aws"
)

var errMockGet = errors.New("get failed")

func TestHandleArchiveUpload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network, func())
		wantErr error
	}{
		{
			name: "uploads a single compressed object",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network, func()) {
				t.Helper()

				source := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(source, "a.txt"), []byte("hello"), 0o600))

				client, mockS3, _ := newMockClient(t)

				var uploaded []byte

				mockS3.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{}, &types.NotFound{}).Once()
				mockS3.On("PutObject", mock.Anything, mock.MatchedBy(func(input *s3.PutObjectInput) bool {
					return awssdk.ToString(input.Key) == "cache/archive.tar.gz" &&
						awssdk.ToString(input.ContentType) == "application/gzip"
				}), mock.Anything, mock.Anything).Return(&s3.PutObjectOutput{}, nil).Once().Run(func(args mock.Arguments) {
					input, ok := args.Get(1).(*s3.PutObjectInput)
					require.True(t, ok)

					body, err := io.ReadAll(input.Body)
					require.NoError(t, err)

					uploaded = body
				})

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:  "bucket",
					Source:  source,
					Target:  "cache/archive.tar.gz",
					Archive: Archive{Enabled: true, Compression: string(archive.CompressionGzip)},
				})

				return p, client, network, func() {
					mockS3.AssertExpectations(t)

					dest := t.TempDir()
					require.NoError(t, archive.Extract(t.Context(), dest, bytes.NewReader(uploaded), archive.CompressionGzip))

					got, err := os.ReadFile(filepath.Join(dest, "a.txt"))
					require.NoError(t, err)
					assert.Equal(t, "hello", string(got))
				}
			},
		},
		{
			name: "skips upload on dry run",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network, func()) {
				t.Helper()

				source := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(source, "a.txt"), []byte("hello"), 0o600))

				client, mockS3, _ := newMockClient(t)
				mockS3.AssertNotCalled(t, "PutObject", mock.Anything, mock.Anything)

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:  "bucket",
					Source:  source,
					Target:  "cache/archive.tar.gz",
					DryRun:  true,
					Archive: Archive{Enabled: true, Compression: string(archive.CompressionGzip)},
				})

				return p, client, network, func() {}
			},
		},
		{
			name: "rejects empty rendered target",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network, func()) {
				t.Helper()

				source := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(source, "a.txt"), []byte("hello"), 0o600))

				client, mockS3, _ := newMockClient(t)
				mockS3.AssertNotCalled(t, "PutObject", mock.Anything, mock.Anything)

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:  "bucket",
					Source:  source,
					Target:  "{{ .Repository.Name }}",
					Archive: Archive{Enabled: true, Compression: string(archive.CompressionGzip)},
				})

				return p, client, network, func() {}
			},
			wantErr: ErrArchiveTargetNotSet,
		},
		{
			name: "rejects empty source directory",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network, func()) {
				t.Helper()

				client, _, _ := newMockClient(t)

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:  "bucket",
					Source:  t.TempDir(),
					Target:  "cache/archive.tar.gz",
					Archive: Archive{Enabled: true, Compression: string(archive.CompressionGzip)},
				})

				return p, client, network, func() {}
			},
			wantErr: ErrEmptySourceDirectory,
		},
		{
			name: "rejects invalid compression",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network, func()) {
				t.Helper()

				client, _, _ := newMockClient(t)

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:  "bucket",
					Source:  t.TempDir(),
					Target:  "cache/archive.tar.gz",
					Archive: Archive{Enabled: true, Compression: "zstd"},
				})

				return p, client, network, func() {}
			},
			wantErr: archive.ErrInvalidCompression,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, client, network, teardown := tt.setup(t)
			defer teardown()

			err := p.handleArchiveUpload(network.Context, *network.Client, plugin_base.Metadata{}, client.S3)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestHandleArchiveDownload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network, func())
		wantErr error
	}{
		{
			name: "downloads and extracts a single object",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network, func()) {
				t.Helper()

				src := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o600))

				var buf bytes.Buffer
				require.NoError(t, archive.Create(t.Context(), src, &buf, archive.CompressionGzip))

				client, mockS3, _ := newMockClient(t)
				mockS3.On("GetObject", mock.Anything, mock.MatchedBy(func(input *s3.GetObjectInput) bool {
					return awssdk.ToString(input.Key) == "cache/archive.tar.gz"
				})).Return(&s3.GetObjectOutput{
					Body: io.NopCloser(bytes.NewReader(buf.Bytes())),
				}, nil).Once()

				dest := t.TempDir()

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:  "bucket",
					Source:  dest,
					Target:  "cache/archive.tar.gz",
					Archive: Archive{Enabled: true, Compression: string(archive.CompressionGzip)},
				})

				return p, client, network, func() {
					mockS3.AssertExpectations(t)

					got, err := os.ReadFile(filepath.Join(dest, "a.txt"))
					require.NoError(t, err)
					assert.Equal(t, "hello", string(got))
				}
			},
		},
		{
			name: "skips download on dry run",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network, func()) {
				t.Helper()

				client, mockS3, _ := newMockClient(t)
				mockS3.AssertNotCalled(t, "GetObject", mock.Anything, mock.Anything)

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:  "bucket",
					Source:  t.TempDir(),
					Target:  "cache/archive.tar.gz",
					DryRun:  true,
					Archive: Archive{Enabled: true, Compression: string(archive.CompressionGzip)},
				})

				return p, client, network, func() {}
			},
		},
		{
			name: "rejects empty rendered target",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network, func()) {
				t.Helper()

				client, mockS3, _ := newMockClient(t)
				mockS3.AssertNotCalled(t, "GetObject", mock.Anything, mock.Anything)

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:  "bucket",
					Source:  t.TempDir(),
					Target:  "{{ .Repository.Name }}",
					Archive: Archive{Enabled: true, Compression: string(archive.CompressionGzip)},
				})

				return p, client, network, func() {}
			},
			wantErr: ErrArchiveTargetNotSet,
		},
		{
			name: "missing object is a no-op",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network, func()) {
				t.Helper()

				client, mockS3, _ := newMockClient(t)
				mockS3.On("GetObject", mock.Anything, mock.Anything).
					Return(&s3.GetObjectOutput{}, &types.NoSuchKey{}).Once()

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:  "bucket",
					Source:  t.TempDir(),
					Target:  "cache/archive.tar.gz",
					Archive: Archive{Enabled: true, Compression: string(archive.CompressionGzip)},
				})

				return p, client, network, func() {
					mockS3.AssertExpectations(t)
				}
			},
		},
		{
			name: "propagates download error",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network, func()) {
				t.Helper()

				client, mockS3, _ := newMockClient(t)
				mockS3.On("GetObject", mock.Anything, mock.Anything).Return(&s3.GetObjectOutput{}, errMockGet)

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:  "bucket",
					Source:  t.TempDir(),
					Target:  "cache/archive.tar.gz",
					Archive: Archive{Enabled: true, Compression: string(archive.CompressionGzip)},
				})

				return p, client, network, func() {}
			},
			wantErr: errMockGet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, client, network, teardown := tt.setup(t)
			defer teardown()

			err := p.handleArchiveDownload(network.Context, *network.Client, plugin_base.Metadata{}, client.S3)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestArchiveContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		compression archive.Compression
		want        string
	}{
		{name: "gzip", compression: archive.CompressionGzip, want: "application/gzip"},
		{name: "none", compression: archive.CompressionNone, want: "application/x-tar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, archiveContentType(tt.compression))
		})
	}
}

func TestRenderTargetStripsLeadingSlash(t *testing.T) {
	t.Parallel()

	p, network := newTestPlugin(t.Context(), &Settings{
		Bucket: "bucket",
		Source: t.TempDir(),
		Target: "{{ .Repository.Branch }}",
	})

	metadata := plugin_base.Metadata{
		Repository: plugin_base.Repository{Branch: "/main"},
	}

	rendered, err := p.renderTarget(network.Context, *network.Client, metadata)
	require.NoError(t, err)
	assert.Equal(t, "main", rendered)
}

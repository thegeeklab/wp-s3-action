package plugin

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	plugin_base "github.com/thegeeklab/wp-plugin-go/v7/plugin"
	"github.com/thegeeklab/wp-s3-action/aws"
	"github.com/thegeeklab/wp-s3-action/aws/mocks"
)

func newMockClient(t *testing.T) (*aws.Client, *mocks.MockS3APIClient, *mocks.MockCloudfrontAPIClient) {
	t.Helper()

	mockS3 := mocks.NewMockS3APIClient(t)
	mockCf := mocks.NewMockCloudfrontAPIClient(t)

	return aws.NewTestClient(mockS3, mockCf), mockS3, mockCf
}

func newTestPlugin(ctx context.Context, s *Settings) (*Plugin, plugin_base.Network) {
	if s == nil {
		s = &Settings{}
	}

	return &Plugin{
		Plugin:   plugin_base.New(plugin_base.Options{}),
		Settings: s,
	}, plugin_base.Network{Context: ctx}
}

var (
	errMockList    = errors.New("list failed")
	errMockPut     = errors.New("put failed")
	errMockDelete  = errors.New("delete failed")
	errMockInvalid = errors.New("invalidate failed")
)

func listResult(keys ...string) *s3.ListObjectsOutput {
	contents := make([]types.Object, 0, len(keys))
	for _, k := range keys {
		contents = append(contents, types.Object{Key: awssdk.String(k)})
	}

	return &s3.ListObjectsOutput{
		Contents:    contents,
		IsTruncated: awssdk.Bool(false),
	}
}

func TestHandleDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network)
		wantErr error
	}{
		{
			name: "delete all keys under target in a single batched call",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network) {
				t.Helper()

				client, mockS3, _ := newMockClient(t)
				mockS3.On("ListObjects", mock.Anything, mock.Anything).
					Return(listResult("blog/a.txt", "blog/b.txt"), nil)
				mockS3.On("DeleteObjects", mock.Anything, mock.MatchedBy(func(input *s3.DeleteObjectsInput) bool {
					return len(input.Delete.Objects) == 2
				})).Return(&s3.DeleteObjectsOutput{}, nil).Once()

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:         "bucket",
					Target:         "blog",
					MaxConcurrency: 2,
				})

				return p, client, network
			},
		},
		{
			name: "empty list produces no delete jobs",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network) {
				t.Helper()

				client, mockS3, _ := newMockClient(t)
				mockS3.On("ListObjects", mock.Anything, mock.Anything).
					Return(listResult(), nil)
				mockS3.AssertNotCalled(t, "DeleteObjects", mock.Anything, mock.Anything)

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:         "bucket",
					Target:         "blog",
					MaxConcurrency: 1,
				})

				return p, client, network
			},
		},
		{
			name: "list error propagates",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network) {
				t.Helper()

				client, mockS3, _ := newMockClient(t)
				mockS3.On("ListObjects", mock.Anything, mock.Anything).
					Return(&s3.ListObjectsOutput{}, errMockList)

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:         "bucket",
					Target:         "blog",
					MaxConcurrency: 1,
				})

				return p, client, network
			},
			wantErr: errMockList,
		},
		{
			name: "sibling-prefix keys are skipped",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network) {
				t.Helper()

				client, mockS3, _ := newMockClient(t)
				mockS3.On("ListObjects", mock.Anything, mock.Anything).
					Return(listResult("blog/a.txt", "blogger/x.txt"), nil)
				mockS3.On("DeleteObjects", mock.Anything, mock.MatchedBy(func(input *s3.DeleteObjectsInput) bool {
					return len(input.Delete.Objects) == 1 && *input.Delete.Objects[0].Key == "blog/a.txt"
				})).Return(&s3.DeleteObjectsOutput{}, nil).Once()

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:         "bucket",
					Target:         "blog",
					MaxConcurrency: 1,
				})

				return p, client, network
			},
		},
		{
			name: "split into chunks at the s3 batch limit",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network) {
				t.Helper()

				keys := make([]string, 0, aws.MaxDeleteBatch+1)
				for i := range aws.MaxDeleteBatch + 1 {
					keys = append(keys, "blog/f"+strconv.Itoa(i)+".txt")
				}

				client, mockS3, _ := newMockClient(t)
				mockS3.On("ListObjects", mock.Anything, mock.Anything).
					Return(listResult(keys...), nil)
				mockS3.On("DeleteObjects", mock.Anything, mock.MatchedBy(func(input *s3.DeleteObjectsInput) bool {
					return len(input.Delete.Objects) <= aws.MaxDeleteBatch
				})).Return(&s3.DeleteObjectsOutput{}, nil).Times(2)

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:         "bucket",
					Target:         "blog",
					MaxConcurrency: 1,
				})

				return p, client, network
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, client, network := tt.setup(t)

			err := p.handleDelete(network, client)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestHandleRedirect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		target    string
		redirects map[string]string
		wantKey   string
		wantErr   error
	}{
		{
			name:      "redirects are namespaced under target",
			target:    "blog",
			redirects: map[string]string{"old": "https://example.com/new"},
			wantKey:   "blog/old",
		},
		{
			name:      "empty target writes at bucket root",
			target:    "",
			redirects: map[string]string{"old": "https://example.com/new"},
			wantKey:   "old",
		},
		{
			name:      "leading slash on redirect key is trimmed",
			target:    "blog",
			redirects: map[string]string{"/old": "https://example.com/new"},
			wantKey:   "blog/old",
		},
		{
			name:      "put object failure propagates",
			target:    "blog",
			redirects: map[string]string{"old": "https://example.com/new"},
			wantErr:   errMockPut,
		},
		{
			name:      "no redirects runs zero jobs",
			target:    "blog",
			redirects: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, mockS3, _ := newMockClient(t)

			switch {
			case tt.wantErr != nil:
				mockS3.On("PutObject", mock.Anything, mock.Anything).
					Return(&s3.PutObjectOutput{}, errMockPut)
			case len(tt.redirects) > 0:
				mockS3.On("PutObject", mock.Anything, mock.MatchedBy(func(input *s3.PutObjectInput) bool {
					return awssdk.ToString(input.Key) == tt.wantKey
				})).Return(&s3.PutObjectOutput{}, nil).Once()
			default:
				mockS3.AssertNotCalled(t, "PutObject", mock.Anything, mock.Anything)
			}

			p, network := newTestPlugin(t.Context(), &Settings{
				Bucket:         "bucket",
				Target:         tt.target,
				MaxConcurrency: 1,
				Redirects:      tt.redirects,
			})

			err := p.handleRedirect(network, client)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestHandleInvalidateCloudFront(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		distribution string
		target       string
		dryRun       bool
		wantPath     string
		wantErr      error
	}{
		{
			name:         "dry run skips invalidation",
			distribution: "E123",
			target:       "blog",
			dryRun:       true,
		},
		{
			name:         "build path from target",
			distribution: "E123",
			target:       "blog",
			wantPath:     "/blog/*",
		},
		{
			name:         "empty target produces root path",
			distribution: "E123",
			target:       "",
			wantPath:     "/*",
		},
		{
			name:         "invalidate failure propagates",
			distribution: "E123",
			target:       "blog",
			wantErr:      errMockInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, _, mockCf := newMockClient(t)

			switch {
			case tt.dryRun:
				mockCf.AssertNotCalled(t, "CreateInvalidation", mock.Anything, mock.Anything)
			case tt.wantErr != nil:
				mockCf.On("CreateInvalidation", mock.Anything, mock.Anything).
					Return(&cloudfront.CreateInvalidationOutput{}, errMockInvalid)
			default:
				mockCf.On("CreateInvalidation", mock.Anything, mock.MatchedBy(func(input *cloudfront.CreateInvalidationInput) bool {
					if input == nil || input.InvalidationBatch == nil || input.InvalidationBatch.Paths == nil {
						return false
					}

					items := input.InvalidationBatch.Paths.Items

					return awssdk.ToString(input.DistributionId) == tt.distribution &&
						len(items) == 1 &&
						items[0] == tt.wantPath
				})).Return(&cloudfront.CreateInvalidationOutput{}, nil).Once()
			}

			p, network := newTestPlugin(t.Context(), &Settings{
				Bucket:     "bucket",
				Target:     tt.target,
				DryRun:     tt.dryRun,
				CloudFront: CloudFront{Distribution: tt.distribution},
			})

			err := p.handleInvalidateCloudFront(network, client)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestRunActionJobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		build      func(t *testing.T, jobs chan<- Job) error
		mockDelete func(t *testing.T, mockS3 *mocks.MockS3APIClient)
		wantErr    error
		errMsg     string
	}{
		{
			name: "build error short-circuits without running",
			build: func(_ *testing.T, _ chan<- Job) error {
				return errMockList
			},
			mockDelete: func(t *testing.T, mockS3 *mocks.MockS3APIClient) {
				t.Helper()
				mockS3.AssertNotCalled(t, "DeleteObjects", mock.Anything, mock.Anything)
			},
			wantErr: errMockList,
		},
		{
			name: "wraps run errors with action label",
			build: func(_ *testing.T, jobs chan<- Job) error {
				jobs <- Job{remoteSet: []string{"x"}, action: S3ActionDelete}

				return nil
			},
			mockDelete: func(t *testing.T, mockS3 *mocks.MockS3APIClient) {
				t.Helper()
				mockS3.On("DeleteObjects", mock.Anything, mock.Anything).
					Return(&s3.DeleteObjectsOutput{}, errMockDelete).Once()
			},
			wantErr: errMockDelete,
			errMsg:  "delete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, mockS3, _ := newMockClient(t)
			tt.mockDelete(t, mockS3)

			p, network := newTestPlugin(t.Context(), &Settings{
				Bucket:         "bucket",
				MaxConcurrency: 1,
			})

			err := p.runActionJobs(network, client, S3ActionDelete, func(jobs chan<- Job) error {
				return tt.build(t, jobs)
			})

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}

				return
			}

			assert.NoError(t, err)
			mockS3.AssertNumberOfCalls(t, "DeleteObjects", 1)
		})
	}
}

func TestValidateSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) *Settings
		wantErr error
		wantAny bool
	}{
		{
			name: "non-empty source succeeds",
			setup: func(t *testing.T) *Settings {
				t.Helper()

				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0o600))

				return &Settings{Source: dir}
			},
		},
		{
			name: "empty source without allow flag fails",
			setup: func(t *testing.T) *Settings {
				t.Helper()

				return &Settings{
					Source: t.TempDir(),
					Upload: Upload{AllowEmptySource: false},
				}
			},
			wantErr: ErrEmptySourceDirectory,
		},
		{
			name: "empty source with allow flag succeeds",
			setup: func(t *testing.T) *Settings {
				t.Helper()

				return &Settings{
					Source: t.TempDir(),
					Upload: Upload{AllowEmptySource: true},
				}
			},
		},
		{
			name: "missing source directory fails",
			setup: func(t *testing.T) *Settings {
				t.Helper()

				return &Settings{
					Source: filepath.Join(t.TempDir(), "does-not-exist"),
				}
			},
			wantAny: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, _ := newTestPlugin(t.Context(), tt.setup(t))

			err := p.validateSource()

			switch {
			case tt.wantErr != nil:
				assert.ErrorIs(t, err, tt.wantErr)
			case tt.wantAny:
				assert.Error(t, err)
			default:
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateUploadJobs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("c"), 0o600))

	p, _ := newTestPlugin(t.Context(), &Settings{
		Source: dir,
		Target: "blog",
	})

	var collected []Job

	yield := func(j Job) error {
		collected = append(collected, j)

		return nil
	}

	local, err := p.createUploadJobs(yield)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"a.txt", "b.txt", "sub/c.txt"}, local)
	assert.Len(t, collected, 3)

	for _, job := range collected {
		assert.Equal(t, S3ActionUpload, job.action)
		assert.True(t, filepath.IsAbs(job.local), "local %q should be absolute", job.local)

		prefix := dir + string(filepath.Separator)
		assert.True(t, strings.HasPrefix(job.local, prefix),
			"local %q must stay inside source %q", job.local, dir)

		assert.Equal(t, filepath.Join("blog", job.local[len(dir)+1:]), job.remote)
	}
}

func TestCreateMirrorDeleteJobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		target     string
		redirects  map[string]string
		local      []string
		remoteKeys []string
		wantDelete []string
		wantJobs   int
	}{
		{
			name:       "no remote keys, no delete jobs",
			target:     "blog",
			local:      []string{"a.txt"},
			remoteKeys: []string{},
			wantDelete: []string{},
			wantJobs:   0,
		},
		{
			name:       "remote key not in local is deleted",
			target:     "blog",
			local:      []string{"a.txt"},
			remoteKeys: []string{"blog/a.txt", "blog/b.txt"},
			wantDelete: []string{"blog/b.txt"},
			wantJobs:   1,
		},
		{
			name:       "redirects protect matching keys from deletion",
			target:     "blog",
			redirects:  map[string]string{"old/path": "https://example.com/new"},
			local:      []string{"a.txt"},
			remoteKeys: []string{"blog/old/path", "blog/a.txt", "blog/b.txt"},
			wantDelete: []string{"blog/b.txt"},
			wantJobs:   1,
		},
		{
			name:       "leading slash on redirect is trimmed",
			target:     "blog",
			redirects:  map[string]string{"/old": "https://example.com/new"},
			local:      []string{},
			remoteKeys: []string{"blog/old"},
			wantDelete: []string{},
			wantJobs:   0,
		},
		{
			name:       "empty target strips no prefix",
			target:     "",
			local:      []string{"a.txt"},
			remoteKeys: []string{"a.txt", "b.txt"},
			wantDelete: []string{"b.txt"},
			wantJobs:   1,
		},
		{
			name:       "trailing slash target strips prefix correctly",
			target:     "blog/",
			local:      []string{"a.txt"},
			remoteKeys: []string{"blog/a.txt"},
			wantDelete: []string{},
			wantJobs:   0,
		},
		{
			name:       "skip sibling-prefix keys that share target string prefix",
			target:     "blog",
			local:      []string{"a.txt"},
			remoteKeys: []string{"blog/a.txt", "blogger/x.txt"},
			wantDelete: []string{},
			wantJobs:   0,
		},
		{
			name:   "batch keys up to the s3 delete limit in a single job",
			target: "blog",
			local:  []string{},
			remoteKeys: func() []string {
				keys := make([]string, 0, aws.MaxDeleteBatch)
				for i := range aws.MaxDeleteBatch {
					keys = append(keys, "blog/f"+strconv.Itoa(i)+".txt")
				}

				return keys
			}(),
			wantDelete: func() []string {
				keys := make([]string, 0, aws.MaxDeleteBatch)
				for i := range aws.MaxDeleteBatch {
					keys = append(keys, "blog/f"+strconv.Itoa(i)+".txt")
				}

				return keys
			}(),
			wantJobs: 1,
		},
		{
			name:   "split mirror delete into chunks beyond the s3 delete limit",
			target: "blog",
			local:  []string{},
			remoteKeys: func() []string {
				keys := make([]string, 0, aws.MaxDeleteBatch+1)
				for i := range aws.MaxDeleteBatch + 1 {
					keys = append(keys, "blog/f"+strconv.Itoa(i)+".txt")
				}

				return keys
			}(),
			wantDelete: func() []string {
				keys := make([]string, 0, aws.MaxDeleteBatch+1)
				for i := range aws.MaxDeleteBatch + 1 {
					keys = append(keys, "blog/f"+strconv.Itoa(i)+".txt")
				}

				return keys
			}(),
			wantJobs: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var collected []Job

			client, mockS3, _ := newMockClient(t)
			mockS3.On("ListObjects", mock.Anything, mock.Anything).
				Return(listResult(tt.remoteKeys...), nil)

			p, _ := newTestPlugin(t.Context(), &Settings{
				Target:    tt.target,
				Redirects: tt.redirects,
			})

			err := p.createMirrorDeleteJobs(t.Context(), client, append([]string{}, tt.local...), func(j Job) error {
				collected = append(collected, j)

				return nil
			})
			assert.NoError(t, err)

			assert.Len(t, collected, tt.wantJobs)

			gotDeletes := make([]string, 0)

			for _, job := range collected {
				assert.Equal(t, S3ActionDelete, job.action)
				assert.LessOrEqual(t, len(job.remoteSet), aws.MaxDeleteBatch)
				gotDeletes = append(gotDeletes, job.remoteSet...)
			}

			assert.ElementsMatch(t, tt.wantDelete, gotDeletes)
		})
	}
}

func TestHandleDownload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network)
		wantErr error
	}{
		{
			name: "download builds one job per remote key",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network) {
				t.Helper()

				client, mockS3, _ := newMockClient(t)
				mockS3.On("ListObjects", mock.Anything, mock.Anything).
					Return(listResult("blog/a.txt", "blog/b.txt"), nil)
				mockS3.On("GetObject", mock.Anything, mock.Anything).
					Return(func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) *s3.GetObjectOutput {
						return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("body"))}
					}, nil).Twice()

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:         "bucket",
					Source:         t.TempDir(),
					Target:         "blog",
					MaxConcurrency: 2,
				})

				return p, client, network
			},
		},
		{
			name: "list failure propagates with context",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network) {
				t.Helper()

				client, mockS3, _ := newMockClient(t)
				mockS3.On("ListObjects", mock.Anything, mock.Anything).
					Return(&s3.ListObjectsOutput{}, errMockList)

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:         "bucket",
					Source:         t.TempDir(),
					Target:         "blog",
					MaxConcurrency: 1,
				})

				return p, client, network
			},
			wantErr: errMockList,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, client, network := tt.setup(t)

			err := p.handleDownload(network, client)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestHandleDownloadFiltersSiblingKeys(t *testing.T) {
	t.Parallel()

	client, mockS3, _ := newMockClient(t)
	mockS3.On("ListObjects", mock.Anything, mock.Anything).
		Return(listResult("blog/a.txt", "blogger/x.txt"), nil)
	mockS3.On("GetObject", mock.Anything, mock.MatchedBy(func(input *s3.GetObjectInput) bool {
		return awssdk.ToString(input.Key) == "blog/a.txt"
	})).
		Return(func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) *s3.GetObjectOutput {
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("body"))}
		}, nil).Once()

	p, network := newTestPlugin(t.Context(), &Settings{
		Bucket:         "bucket",
		Source:         t.TempDir(),
		Target:         "blog",
		MaxConcurrency: 1,
	})

	err := p.handleDownload(network, client)
	assert.NoError(t, err)
	mockS3.AssertExpectations(t)
}

func TestHandleUpload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network)
		wantErr error
	}{
		{
			name: "uploads files from source to target",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network) {
				t.Helper()

				source := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(source, "a.txt"), []byte("a"), 0o600))
				require.NoError(t, os.WriteFile(filepath.Join(source, "b.txt"), []byte("b"), 0o600))

				client, mockS3, _ := newMockClient(t)
				mockS3.On("HeadObject", mock.Anything, mock.Anything).
					Return(&s3.HeadObjectOutput{}, &types.NotFound{})
				mockS3.On("PutObject", mock.Anything, mock.Anything).
					Return(&s3.PutObjectOutput{}, nil)

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket:         "bucket",
					Source:         source,
					Target:         "blog",
					MaxConcurrency: 2,
				})

				return p, client, network
			},
		},
		{
			name: "empty source fails when allow flag is off",
			setup: func(t *testing.T) (*Plugin, *aws.Client, plugin_base.Network) {
				t.Helper()

				client, _, _ := newMockClient(t)

				p, network := newTestPlugin(t.Context(), &Settings{
					Bucket: "bucket",
					Source: t.TempDir(),
					Upload: Upload{AllowEmptySource: false},
				})

				return p, client, network
			},
			wantErr: ErrEmptySourceDirectory,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, client, network := tt.setup(t)

			err := p.handleUpload(network, client)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestEmitDeleteBatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		initial   []string
		yieldErr  error
		wantJob   *Job
		wantBatch []string
	}{
		{
			name:    "no-op when batch is empty",
			initial: nil,
		},
		{
			name:      "yields a single delete job and resets the batch",
			initial:   []string{"a", "b", "c"},
			wantJob:   &Job{remoteSet: []string{"a", "b", "c"}, action: S3ActionDelete},
			wantBatch: []string{},
		},
		{
			name:     "propagates yield error and still resets the batch",
			initial:  []string{"x"},
			yieldErr: errMockDelete,
			wantJob:  &Job{remoteSet: []string{"x"}, action: S3ActionDelete},
			// batch is reset before yield returns, so it is empty
			// even when the consumer errors out
			wantBatch: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			batch := append([]string(nil), tt.initial...)

			var got *Job

			err := emitDeleteBatch(&batch, func(j Job) error {
				got = &j

				return tt.yieldErr
			})

			if tt.wantJob == nil {
				assert.NoError(t, err)
				assert.Nil(t, got)

				return
			}

			assert.Equal(t, tt.wantJob, got)
			assert.Equal(t, tt.wantBatch, batch)

			if tt.yieldErr != nil {
				assert.ErrorIs(t, err, tt.yieldErr)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestJobChannelYield(t *testing.T) {
	t.Parallel()

	t.Run("forwards job to channel", func(t *testing.T) {
		t.Parallel()

		jobs := make(chan Job, 1)
		yield := jobChannelYield(t.Context(), jobs)

		err := yield(Job{remote: "k", action: S3ActionDownload})
		assert.NoError(t, err)

		got := <-jobs
		assert.Equal(t, "k", got.remote)
		assert.Equal(t, S3ActionDownload, got.action)
	})

	t.Run("returns context error when consumer is gone", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())

		jobs := make(chan Job)

		yield := jobChannelYield(ctx, jobs)

		cancel()

		err := yield(Job{remote: "k", action: S3ActionDownload})
		assert.ErrorIs(t, err, context.Canceled)
	})
}

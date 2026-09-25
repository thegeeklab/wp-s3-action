package aws

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/thegeeklab/wp-s3-action/aws/mocks"
)

var (
	ErrPutObject    = errors.New("put object failed")
	ErrDeleteObject = errors.New("delete object failed")
	ErrListObjects  = errors.New("list objects failed")
	ErrGetObject    = errors.New("get object failed")
	ErrCloseFile    = errors.New("close file failed")
	errAny          = errors.New("any error")
	errMockAbort    = errors.New("abort iteration")
)

type failingCloseWriter struct {
	err error
}

func (w *failingCloseWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (w *failingCloseWriter) Close() error {
	return w.err
}

type failingReader struct {
	err error
}

func (r *failingReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

func createTempFile(t *testing.T, name string) string {
	t.Helper()

	name = filepath.Join(t.TempDir(), name)
	_ = os.WriteFile(name, []byte("hello"), 0o600)

	return name
}

// newMetadataUpdateMock returns an S3 API client whose HeadObject reports a
// matching content hash so Upload falls through to the CopyObject metadata
// update path.
func newMetadataUpdateMock(t *testing.T, head *s3.HeadObjectOutput) *mocks.MockS3APIClient {
	t.Helper()

	mockS3Client := mocks.NewMockS3APIClient(t)
	mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(head, nil)
	mockS3Client.On("CopyObject", mock.Anything, mock.Anything).Return(&s3.CopyObjectOutput{}, nil)

	return mockS3Client
}

func TestS3_Upload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T) (*S3, S3UploadOptions, func())
		wantResult UploadResult
		wantErr    error
	}{
		{
			name: "skip upload when local is empty",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				return &S3{},
					S3UploadOptions{
						LocalFilePath: "",
					}, func() {}
			},
			wantResult: UploadResultSkipped,
			wantErr:    nil,
		},
		{
			name: "error when local file does not exist",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				return &S3{},
					S3UploadOptions{
						LocalFilePath: "/path/to/non-existent/file",
					}, func() {}
			},
			wantErr: errAny,
		},
		{
			name: "upload new file with default acl and content type",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{}, &types.NotFound{})
				mockS3Client.On("PutObject", mock.Anything, mock.Anything).Return(&s3.PutObjectOutput{}, nil)

				svc := &S3{
					client: mockS3Client,
					Bucket: "test-bucket",
				}

				uploadOpts := S3UploadOptions{
					LocalFilePath:   createTempFile(t, "file.txt"),
					RemoteObjectKey: "remote/path/file.txt",
				}

				return svc, uploadOpts, func() {
					mockS3Client.AssertExpectations(t)
				}
			},
			wantResult: UploadResultAdded,
			wantErr:    nil,
		},
		{
			name: "update metadata when content type changed",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				mockS3Client := newMetadataUpdateMock(t, &s3.HeadObjectOutput{
					ETag:        aws.String("'5d41402abc4b2a76b9719d911017c592'"),
					ContentType: aws.String("application/octet-stream"),
				})

				return &S3{client: mockS3Client, Bucket: "test-bucket"},
					S3UploadOptions{
						LocalFilePath:   createTempFile(t, "file.txt"),
						RemoteObjectKey: "remote/path/file.txt",
						ContentType:     map[string]string{"*.txt": "text/plain"},
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantResult: UploadResultUpdated,
			wantErr:    nil,
		},
		{
			name: "update metadata when acl changed",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{
					ETag:        aws.String("'5d41402abc4b2a76b9719d911017c592'"),
					ContentType: aws.String("text/plain; charset=utf-8"),
				}, nil)
				mockS3Client.On("GetObjectAcl", mock.Anything, mock.Anything).Return(&s3.GetObjectAclOutput{
					Grants: []types.Grant{
						{
							Grantee: &types.Grantee{
								URI: aws.String("http://acs.amazonaws.com/groups/global/AllUsers"),
							},
							Permission: types.PermissionWrite,
						},
					},
				}, nil)
				mockS3Client.On("CopyObject", mock.Anything, mock.Anything).Return(&s3.CopyObjectOutput{}, nil)

				svc := &S3{
					client: mockS3Client,
					Bucket: "test-bucket",
				}

				uploadOpts := S3UploadOptions{
					LocalFilePath:   createTempFile(t, "file.txt"),
					RemoteObjectKey: "remote/path/file.txt",
					ACL:             map[string]string{"*.txt": "public-read"},
				}

				return svc, uploadOpts, func() {
					mockS3Client.AssertExpectations(t)
				}
			},
			wantResult: UploadResultUpdated,
			wantErr:    nil,
		},
		{
			name: "update metadata when cache control changed",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				mockS3Client := newMetadataUpdateMock(t, &s3.HeadObjectOutput{
					ETag:         aws.String("'5d41402abc4b2a76b9719d911017c592'"),
					ContentType:  aws.String("text/plain; charset=utf-8"),
					CacheControl: aws.String("max-age=0"),
				})

				return &S3{client: mockS3Client, Bucket: "test-bucket"},
					S3UploadOptions{
						LocalFilePath:   createTempFile(t, "file.txt"),
						RemoteObjectKey: "remote/path/file.txt",
						CacheControl:    map[string]string{"*.txt": "max-age=3600"},
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantResult: UploadResultUpdated,
			wantErr:    nil,
		},
		{
			name: "update metadata when content encoding changed",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				mockS3Client := newMetadataUpdateMock(t, &s3.HeadObjectOutput{
					ETag:            aws.String("'5d41402abc4b2a76b9719d911017c592'"),
					ContentType:     aws.String("text/plain; charset=utf-8"),
					ContentEncoding: aws.String("identity"),
				})

				return &S3{client: mockS3Client, Bucket: "test-bucket"},
					S3UploadOptions{
						LocalFilePath:   createTempFile(t, "file.txt"),
						RemoteObjectKey: "remote/path/file.txt",
						ContentEncoding: map[string]string{"*.txt": "gzip"},
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantResult: UploadResultUpdated,
			wantErr:    nil,
		},
		{
			name: "update metadata when metadata changed",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				mockS3Client := newMetadataUpdateMock(t, &s3.HeadObjectOutput{
					ETag:        aws.String("'5d41402abc4b2a76b9719d911017c592'"),
					ContentType: aws.String("text/plain; charset=utf-8"),
					Metadata:    map[string]string{"key": "old-value"},
				})

				return &S3{client: mockS3Client, Bucket: "test-bucket"},
					S3UploadOptions{
						LocalFilePath:   createTempFile(t, "file.txt"),
						RemoteObjectKey: "remote/path/file.txt",
						Metadata:        map[string]map[string]string{"*.txt": {"key": "value"}},
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantResult: UploadResultUpdated,
			wantErr:    nil,
		},
		{
			name: "upload new file when dry run is true",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{}, &types.NotFound{})

				svc := &S3{
					client: mockS3Client,
					Bucket: "test-bucket",
					DryRun: true,
				}

				uploadOpts := S3UploadOptions{
					LocalFilePath:   createTempFile(t, "file1.txt"),
					RemoteObjectKey: "remote/path/file1.txt",
				}

				return svc, uploadOpts, func() {
					mockS3Client.AssertExpectations(t)
				}
			},
			wantResult: UploadResultAdded,
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, opt, teardown := tt.setup(t)
			defer teardown()

			result, err := svc.Upload(t.Context(), opt)
			if tt.wantErr != nil {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func TestS3_Redirect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (*S3, S3RedirectOptions, func())
		wantErr error
	}{
		{
			name: "redirect with valid options",
			setup: func(t *testing.T) (*S3, S3RedirectOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("PutObject", mock.Anything, mock.Anything).Return(&s3.PutObjectOutput{}, nil)

				svc := &S3{
					client: mockS3Client,
					Bucket: "test-bucket",
				}

				redirectOpts := S3RedirectOptions{
					Path:     "redirect/path",
					Location: "https://example.com",
				}

				return svc, redirectOpts, func() {
					mockS3Client.AssertExpectations(t)
				}
			},
			wantErr: nil,
		},
		{
			name: "skip redirect when dry run is true",
			setup: func(t *testing.T) (*S3, S3RedirectOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)

				svc := &S3{
					client: mockS3Client,
					Bucket: "test-bucket",
					DryRun: true,
				}

				redirectOpts := S3RedirectOptions{
					Path:     "redirect/path",
					Location: "https://example.com",
				}

				return svc, redirectOpts, func() {
					mockS3Client.AssertExpectations(t)
				}
			},
			wantErr: nil,
		},
		{
			name: "error when put object fails",
			setup: func(t *testing.T) (*S3, S3RedirectOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.
					On("PutObject", mock.Anything, mock.Anything).
					Return(&s3.PutObjectOutput{}, ErrPutObject)

				svc := &S3{
					client: mockS3Client,
					Bucket: "test-bucket",
				}

				redirectOpts := S3RedirectOptions{
					Path:     "redirect/path",
					Location: "https://example.com",
				}

				return svc, redirectOpts, func() {
					mockS3Client.AssertExpectations(t)
				}
			},
			wantErr: errAny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, opt, teardown := tt.setup(t)
			defer teardown()

			err := svc.Redirect(t.Context(), opt)
			if tt.wantErr != nil {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestS3_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		setup           func(t *testing.T) (*S3, S3DeleteOptions, func())
		wantErr         error
		wantErrContains []string
	}{
		{
			name: "skip when keys are empty",
			setup: func(t *testing.T) (*S3, S3DeleteOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.AssertNotCalled(t, "DeleteObjects", mock.Anything, mock.Anything)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3DeleteOptions{RemoteObjectKeys: nil},
					func() {}
			},
			wantErr: nil,
		},
		{
			name: "skip when dry run is true",
			setup: func(t *testing.T) (*S3, S3DeleteOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.AssertNotCalled(t, "DeleteObjects", mock.Anything, mock.Anything)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
						DryRun: true,
					},
					S3DeleteOptions{RemoteObjectKeys: []string{"a.txt", "b.txt"}},
					func() {}
			},
			wantErr: nil,
		},
		{
			name: "send all keys in a single delete objects call",
			setup: func(t *testing.T) (*S3, S3DeleteOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("DeleteObjects", mock.Anything, mock.MatchedBy(func(input *s3.DeleteObjectsInput) bool {
					return len(input.Delete.Objects) == 2
				})).Return(&s3.DeleteObjectsOutput{}, nil).Once()

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3DeleteOptions{RemoteObjectKeys: []string{"a.txt", "b.txt"}},
					func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: nil,
		},
		{
			name: "propagate delete objects error",
			setup: func(t *testing.T) (*S3, S3DeleteOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("DeleteObjects", mock.Anything, mock.Anything).
					Return(&s3.DeleteObjectsOutput{}, ErrDeleteObject)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3DeleteOptions{RemoteObjectKeys: []string{"a.txt"}},
					func() {}
			},
			wantErr: errAny,
		},
		{
			name: "surfaces per-key delete failures from the response body",
			setup: func(t *testing.T) (*S3, S3DeleteOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("DeleteObjects", mock.Anything, mock.Anything).
					Return(&s3.DeleteObjectsOutput{
						Errors: []types.Error{
							{
								Key:     aws.String("a.txt"),
								Code:    aws.String("AccessDenied"),
								Message: aws.String("Access Denied"),
							},
							{
								Key:     aws.String("b.txt"),
								Code:    aws.String("AccessDenied"),
								Message: aws.String("Access Denied"),
							},
						},
					}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3DeleteOptions{RemoteObjectKeys: []string{"a.txt", "b.txt"}},
					func() {}
			},
			wantErr:         ErrPartialDelete,
			wantErrContains: []string{"a.txt", "b.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, opt, teardown := tt.setup(t)
			defer teardown()

			err := svc.Delete(t.Context(), opt)
			if tt.wantErr != nil {
				assert.Error(t, err)

				for _, want := range tt.wantErrContains {
					assert.Contains(t, err.Error(), want)
				}

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestS3_Download(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (*S3, S3DownloadOptions, func())
		wantErr error
	}{
		{
			name: "skip download when remote key is empty",
			setup: func(t *testing.T) (*S3, S3DownloadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3DownloadOptions{
						LocalFilePath:   filepath.Join(t.TempDir(), "file.txt"),
						RemoteObjectKey: "",
					},
					func() {
						mockS3Client.AssertNotCalled(t, "GetObject", mock.Anything, mock.Anything)
					}
			},
			wantErr: nil,
		},
		{
			name: "skip download when remote key is a directory marker",
			setup: func(t *testing.T) (*S3, S3DownloadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3DownloadOptions{
						LocalFilePath:   filepath.Join(t.TempDir(), "dir"),
						RemoteObjectKey: "prefix/dir/",
					},
					func() {
						mockS3Client.AssertNotCalled(t, "GetObject", mock.Anything, mock.Anything)
					}
			},
			wantErr: nil,
		},
		{
			name: "download object to local file",
			setup: func(t *testing.T) (*S3, S3DownloadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("GetObject", mock.Anything, mock.Anything).Return(&s3.GetObjectOutput{
					Body: io.NopCloser(strings.NewReader("hello")),
				}, nil)

				dest := filepath.Join(t.TempDir(), "sub", "file.txt")

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3DownloadOptions{
						LocalFilePath:   dest,
						RemoteObjectKey: "prefix/file.txt",
					},
					func() {
						mockS3Client.AssertExpectations(t)

						got, err := os.ReadFile(dest)
						assert.NoError(t, err)
						assert.Equal(t, "hello", string(got))
					}
			},
			wantErr: nil,
		},
		{
			name: "error when get object fails",
			setup: func(t *testing.T) (*S3, S3DownloadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.
					On("GetObject", mock.Anything, mock.Anything).
					Return(&s3.GetObjectOutput{}, ErrGetObject)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3DownloadOptions{
						LocalFilePath:   filepath.Join(t.TempDir(), "file.txt"),
						RemoteObjectKey: "prefix/file.txt",
					},
					func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: errAny,
		},
		{
			name: "error when closing local file fails",
			setup: func(t *testing.T) (*S3, S3DownloadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("GetObject", mock.Anything, mock.Anything).Return(&s3.GetObjectOutput{
					Body: io.NopCloser(strings.NewReader("hello")),
				}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
						createFile: func(string) (io.WriteCloser, error) {
							return &failingCloseWriter{err: ErrCloseFile}, nil
						},
					},
					S3DownloadOptions{
						LocalFilePath:   filepath.Join(t.TempDir(), "file.txt"),
						RemoteObjectKey: "prefix/file.txt",
					},
					func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: errAny,
		},
		{
			name: "reject empty local file path",
			setup: func(t *testing.T) (*S3, S3DownloadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.AssertNotCalled(t, "GetObject", mock.Anything, mock.Anything)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3DownloadOptions{
						LocalFilePath:   "",
						RemoteObjectKey: "prefix/file.txt",
					},
					func() {}
			},
			wantErr: errAny,
		},
		{
			name: "preserve existing file when body read fails",
			setup: func(t *testing.T) (*S3, S3DownloadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("GetObject", mock.Anything, mock.Anything).Return(&s3.GetObjectOutput{
					Body: io.NopCloser(&failingReader{err: ErrGetObject}),
				}, nil)

				dest := filepath.Join(t.TempDir(), "file.txt")
				original := []byte("original-content")
				require.NoError(t, os.WriteFile(dest, original, 0o600))

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3DownloadOptions{
						LocalFilePath:   dest,
						RemoteObjectKey: "prefix/file.txt",
					},
					func() {
						mockS3Client.AssertExpectations(t)

						got, err := os.ReadFile(dest)
						assert.NoError(t, err)
						assert.Equal(t, original, got)
					}
			},
			wantErr: errAny,
		},
		{
			name: "reject download when intermediate is a symlink that escapes LocalRoot",
			setup: func(t *testing.T) (*S3, S3DownloadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("GetObject", mock.Anything, mock.Anything).Return(&s3.GetObjectOutput{
					Body: io.NopCloser(strings.NewReader("hello")),
				}, nil)

				root := t.TempDir()
				outside := t.TempDir()
				require.NoError(t, os.Symlink(outside, filepath.Join(root, "escape")))

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3DownloadOptions{
						LocalRoot:       root,
						LocalFilePath:   filepath.Join(root, "escape", "file.txt"),
						RemoteObjectKey: "prefix/file.txt",
					},
					func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: ErrLocalPathOutsideRoot,
		},
		{
			name: "allow download when LocalRoot is empty (skip re-check)",
			setup: func(t *testing.T) (*S3, S3DownloadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("GetObject", mock.Anything, mock.Anything).Return(&s3.GetObjectOutput{
					Body: io.NopCloser(strings.NewReader("hello")),
				}, nil)

				dest := filepath.Join(t.TempDir(), "file.txt")

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3DownloadOptions{
						LocalRoot:       "",
						LocalFilePath:   dest,
						RemoteObjectKey: "prefix/file.txt",
					},
					func() {
						got, err := os.ReadFile(dest)
						assert.NoError(t, err)
						assert.Equal(t, "hello", string(got))
					}
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, opt, teardown := tt.setup(t)
			defer teardown()

			err := svc.Download(t.Context(), opt)
			if tt.wantErr != nil {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestS3_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (*S3, S3ListOptions, func())
		abortOn string // empty means never abort
		wantErr error
		want    []string
	}{
		{
			name: "list objects in prefix",
			setup: func(t *testing.T) (*S3, S3ListOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("ListObjects", mock.Anything, mock.Anything).Return(&s3.ListObjectsOutput{
					Contents: []types.Object{
						{Key: aws.String("prefix/file1.txt")},
						{Key: aws.String("prefix/file2.txt")},
					},
					IsTruncated: aws.Bool(false),
				}, nil)

				svc := &S3{
					client: mockS3Client,
					Bucket: "test-bucket",
				}

				listOpts := S3ListOptions{
					Path: "prefix/",
				}

				return svc, listOpts, func() {
					mockS3Client.AssertExpectations(t)
				}
			},
			want: []string{"prefix/file1.txt", "prefix/file2.txt"},
		},
		{
			name: "list objects with pagination",
			setup: func(t *testing.T) (*S3, S3ListOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("ListObjects", mock.Anything, mock.MatchedBy(func(input *s3.ListObjectsInput) bool {
					return input.Marker == nil
				})).Return(&s3.ListObjectsOutput{
					Contents: []types.Object{
						{Key: aws.String("prefix/file1.txt")},
						{Key: aws.String("prefix/file2.txt")},
					},
					IsTruncated: aws.Bool(true),
				}, nil)
				mockS3Client.On("ListObjects", mock.Anything, mock.MatchedBy(func(input *s3.ListObjectsInput) bool {
					return *input.Marker == "prefix/file2.txt"
				})).Return(&s3.ListObjectsOutput{
					Contents: []types.Object{
						{Key: aws.String("prefix/file3.txt")},
					},
					IsTruncated: aws.Bool(false),
				}, nil)

				svc := &S3{
					client: mockS3Client,
					Bucket: "test-bucket",
				}

				listOpts := S3ListOptions{
					Path: "prefix/",
				}

				return svc, listOpts, func() {
					mockS3Client.AssertExpectations(t)
				}
			},
			want: []string{"prefix/file1.txt", "prefix/file2.txt", "prefix/file3.txt"},
		},
		{
			name: "aborts iteration when callback returns an error",
			setup: func(t *testing.T) (*S3, S3ListOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("ListObjects", mock.Anything, mock.Anything).Return(&s3.ListObjectsOutput{
					Contents: []types.Object{
						{Key: aws.String("a")},
						{Key: aws.String("b")},
					},
					IsTruncated: aws.Bool(false),
				}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3ListOptions{Path: ""},
					func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			abortOn: "a",
			wantErr: errMockAbort,
			want:    []string{"a"},
		},
		{
			name: "propagates list errors",
			setup: func(t *testing.T) (*S3, S3ListOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("ListObjects", mock.Anything, mock.Anything).
					Return(&s3.ListObjectsOutput{}, ErrListObjects)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3ListOptions{Path: ""},
					func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: ErrListObjects,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, opt, teardown := tt.setup(t)
			defer teardown()

			var got []string

			err := svc.List(t.Context(), opt, func(key string) error {
				got = append(got, key)

				if tt.abortOn != "" && key == tt.abortOn {
					return errMockAbort
				}

				return nil
			})

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, tt.want, got)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestS3_ListPaginationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (*S3, S3ListOptions, func())
		wantErr error
	}{
		{
			name: "treats nil IsTruncated as end of listing",
			setup: func(t *testing.T) (*S3, S3ListOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("ListObjects", mock.Anything, mock.Anything).Return(&s3.ListObjectsOutput{
					Contents: []types.Object{
						{Key: aws.String("a")},
					},
					IsTruncated: nil,
				}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3ListOptions{Path: ""},
					func() {
						mockS3Client.AssertNumberOfCalls(t, "ListObjects", 1)
					}
			},
		},
		{
			name: "rejects IsTruncated with empty page",
			setup: func(t *testing.T) (*S3, S3ListOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("ListObjects", mock.Anything, mock.Anything).Return(&s3.ListObjectsOutput{
					Contents:    []types.Object{},
					IsTruncated: aws.Bool(true),
				}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					},
					S3ListOptions{Path: ""},
					func() {
						mockS3Client.AssertNumberOfCalls(t, "ListObjects", 1)
					}
			},
			wantErr: ErrListPagination,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, opt, teardown := tt.setup(t)
			defer teardown()

			err := svc.List(t.Context(), opt, func(string) error { return nil })
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestParseETag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantHash  string
		wantMulti bool
	}{
		{name: "unquoted simple", input: "abc123", wantHash: "abc123"},
		{name: "single-quoted simple", input: "'abc123'", wantHash: "abc123"},
		{name: "double-quoted simple", input: `"abc123"`, wantHash: "abc123"},
		{name: "unquoted multipart", input: "abc123-3", wantHash: "abc123", wantMulti: true},
		{name: "double-quoted multipart", input: `"abc123-7"`, wantHash: "abc123", wantMulti: true},
		{name: "multipart with hyphen in body (split on first -)", input: `"ab-cd-2"`, wantHash: "ab", wantMulti: true},
		{name: "empty string", input: "", wantHash: ""},
		{name: "only quotes", input: `""`, wantHash: ""},
		{name: "nil-like dashes", input: "-3", wantHash: "", wantMulti: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hash, multi := parseETag(tt.input)
			assert.Equal(t, tt.wantHash, hash)
			assert.Equal(t, tt.wantMulti, multi)
		})
	}
}

func TestS3_UploadETagForms(t *testing.T) {
	t.Parallel()

	// Local file content is "hello", MD5 = 5d41402abc4b2a76b9719d911017c592.

	tests := []struct {
		name          string
		mockETag      *string
		wantPutCalls  int
		wantCopyCalls int
	}{
		{
			name:          "double-quoted simple ETag with matching body is treated as in-sync",
			mockETag:      aws.String(`"5d41402abc4b2a76b9719d911017c592"`),
			wantCopyCalls: 0,
			wantPutCalls:  0,
		},
		{
			name:          "multipart ETag falls through to full PutObject",
			mockETag:      aws.String(`"5d41402abc4b2a76b9719d911017c592-3"`),
			wantCopyCalls: 0,
			wantPutCalls:  1,
		},
		{
			name:          "nil ETag does not panic and falls through to PutObject",
			mockETag:      nil,
			wantCopyCalls: 0,
			wantPutCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockS3Client := mocks.NewMockS3APIClient(t)

			mockS3Client.On("HeadObject", mock.Anything, mock.Anything).
				Return(&s3.HeadObjectOutput{
					ETag:        tt.mockETag,
					ContentType: aws.String("text/plain; charset=utf-8"),
				}, nil)
			mockS3Client.On("GetObjectAcl", mock.Anything, mock.Anything).
				Return(&s3.GetObjectAclOutput{}, nil).Maybe()

			if tt.wantPutCalls > 0 {
				mockS3Client.On("PutObject", mock.Anything, mock.Anything).
					Return(&s3.PutObjectOutput{}, nil).Times(tt.wantPutCalls)
			}

			if tt.wantCopyCalls > 0 {
				mockS3Client.On("CopyObject", mock.Anything, mock.Anything).
					Return(&s3.CopyObjectOutput{}, nil).Times(tt.wantCopyCalls)
			}

			svc := &S3{
				client: mockS3Client,
				Bucket: "test-bucket",
			}

			_, err := svc.Upload(t.Context(), S3UploadOptions{
				LocalFilePath:   createTempFile(t, "hello.txt"),
				RemoteObjectKey: "remote/hello.txt",
			})

			assert.NoError(t, err)
			mockS3Client.AssertExpectations(t)
		})
	}
}

func TestS3_UploadCopySourceEncoding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		remoteKey      string
		wantCopySource string
	}{
		{
			name:           "plain key is left unchanged",
			remoteKey:      "remote/path/file.txt",
			wantCopySource: "test-bucket/remote/path/file.txt",
		},
		{
			name:           "space in key is percent-encoded",
			remoteKey:      "remote/path/my file.txt",
			wantCopySource: "test-bucket/remote/path/my%20file.txt",
		},
		{
			name:           "reserved characters are percent-encoded and slashes preserved",
			remoteKey:      "remote/path/a#b?.txt",
			wantCopySource: "test-bucket/remote/path/a%23b%3F.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockS3Client := mocks.NewMockS3APIClient(t)
			mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{
				ETag:        aws.String(`"5d41402abc4b2a76b9719d911017c592"`),
				ContentType: aws.String("application/octet-stream"),
			}, nil)

			var copySource string

			mockS3Client.On("CopyObject", mock.Anything, mock.MatchedBy(func(input *s3.CopyObjectInput) bool {
				copySource = aws.ToString(input.CopySource)

				return true
			})).Return(&s3.CopyObjectOutput{}, nil)

			svc := &S3{client: mockS3Client, Bucket: "test-bucket"}

			_, err := svc.Upload(t.Context(), S3UploadOptions{
				LocalFilePath:   createTempFile(t, "file.txt"),
				RemoteObjectKey: tt.remoteKey,
				ContentType:     map[string]string{"*.txt": "text/plain"},
			})

			require.NoError(t, err)
			assert.Equal(t, tt.wantCopySource, copySource)
		})
	}
}

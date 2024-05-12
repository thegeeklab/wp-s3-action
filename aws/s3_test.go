package aws

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/thegeeklab/wp-s3-action/aws/mocks"
)

func createTempFile(t *testing.T, name string) string {
	t.Helper()

	name = filepath.Join(t.TempDir(), name)
	_ = os.WriteFile(name, []byte("hello"), 0o600)

	return name
}

func TestS3_Upload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (*S3, S3UploadOptions, func())
		wantErr bool
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
			wantErr: false,
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
			wantErr: true,
		},
		{
			name: "upload new file with default acl and content type",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{}, &types.NoSuchKey{})
				mockS3Client.On("PutObject", mock.Anything, mock.Anything).Return(&s3.PutObjectOutput{}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3UploadOptions{
						LocalFilePath:   createTempFile(t, "file.txt"),
						RemoteObjectKey: "remote/path/file.txt",
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
		},
		{
			name: "update metadata when content type changed",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{
					ETag:        aws.String("'5d41402abc4b2a76b9719d911017c592'"),
					ContentType: aws.String("application/octet-stream"),
				}, nil)
				mockS3Client.On("CopyObject", mock.Anything, mock.Anything).Return(&s3.CopyObjectOutput{}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3UploadOptions{
						LocalFilePath:   createTempFile(t, "file.txt"),
						RemoteObjectKey: "remote/path/file.txt",
						ContentType:     map[string]string{"*.txt": "text/plain"},
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
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

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3UploadOptions{
						LocalFilePath:   createTempFile(t, "file.txt"),
						RemoteObjectKey: "remote/path/file.txt",
						ACL:             map[string]string{"*.txt": "public-read"},
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
		},
		{
			name: "update metadata when cache control changed",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{
					ETag:         aws.String("'5d41402abc4b2a76b9719d911017c592'"),
					ContentType:  aws.String("text/plain; charset=utf-8"),
					CacheControl: aws.String("max-age=0"),
				}, nil)
				mockS3Client.On("CopyObject", mock.Anything, mock.Anything).Return(&s3.CopyObjectOutput{}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3UploadOptions{
						LocalFilePath:   createTempFile(t, "file.txt"),
						RemoteObjectKey: "remote/path/file.txt",
						CacheControl:    map[string]string{"*.txt": "max-age=3600"},
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
		},
		{
			name: "update metadata when content encoding changed",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{
					ETag:            aws.String("'5d41402abc4b2a76b9719d911017c592'"),
					ContentType:     aws.String("text/plain; charset=utf-8"),
					ContentEncoding: aws.String("identity"),
				}, nil)
				mockS3Client.On("CopyObject", mock.Anything, mock.Anything).Return(&s3.CopyObjectOutput{}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3UploadOptions{
						LocalFilePath:   createTempFile(t, "file.txt"),
						RemoteObjectKey: "remote/path/file.txt",
						ContentEncoding: map[string]string{"*.txt": "gzip"},
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
		},
		{
			name: "update metadata when metadata changed",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{
					ETag:        aws.String("'5d41402abc4b2a76b9719d911017c592'"),
					ContentType: aws.String("text/plain; charset=utf-8"),
					Metadata:    map[string]string{"key": "old-value"},
				}, nil)
				mockS3Client.On("CopyObject", mock.Anything, mock.Anything).Return(&s3.CopyObjectOutput{}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3UploadOptions{
						LocalFilePath:   createTempFile(t, "file.txt"),
						RemoteObjectKey: "remote/path/file.txt",
						Metadata:        map[string]map[string]string{"*.txt": {"key": "value"}},
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
		},
		{
			name: "upload new file when dry run is true",
			setup: func(t *testing.T) (*S3, S3UploadOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{}, &types.NoSuchKey{})

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
						DryRun: true,
					}, S3UploadOptions{
						LocalFilePath:   createTempFile(t, "file1.txt"),
						RemoteObjectKey: "remote/path/file1.txt",
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s3, opt, teardown := tt.setup(t)
			defer teardown()

			err := s3.Upload(context.Background(), opt)
			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestS3_Redirect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (*S3, S3RedirectOptions, func())
		wantErr bool
	}{
		{
			name: "redirect with valid options",
			setup: func(t *testing.T) (*S3, S3RedirectOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("PutObject", mock.Anything, mock.Anything).Return(&s3.PutObjectOutput{}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3RedirectOptions{
						Path:     "redirect/path",
						Location: "https://example.com",
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
		},
		{
			name: "skip redirect when dry run is true",
			setup: func(t *testing.T) (*S3, S3RedirectOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
						DryRun: true,
					}, S3RedirectOptions{
						Path:     "redirect/path",
						Location: "https://example.com",
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
		},
		{
			name: "error when put object fails",
			setup: func(t *testing.T) (*S3, S3RedirectOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.
					On("PutObject", mock.Anything, mock.Anything).
					Return(&s3.PutObjectOutput{}, errors.New("put object failed"))

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3RedirectOptions{
						Path:     "redirect/path",
						Location: "https://example.com",
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s3, opt, teardown := tt.setup(t)
			defer teardown()

			err := s3.Redirect(context.Background(), opt)
			if tt.wantErr {
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
		name    string
		setup   func(t *testing.T) (*S3, S3DeleteOptions, func())
		wantErr bool
	}{
		{
			name: "delete existing object",
			setup: func(t *testing.T) (*S3, S3DeleteOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("DeleteObject", mock.Anything, mock.Anything).Return(&s3.DeleteObjectOutput{}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3DeleteOptions{
						RemoteObjectKey: "path/to/file.txt",
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
		},
		{
			name: "skip delete when dry run is true",
			setup: func(t *testing.T) (*S3, S3DeleteOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
						DryRun: true,
					}, S3DeleteOptions{
						RemoteObjectKey: "path/to/file.txt",
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
		},
		{
			name: "error when delete object fails",
			setup: func(t *testing.T) (*S3, S3DeleteOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.
					On("DeleteObject", mock.Anything, mock.Anything).
					Return(&s3.DeleteObjectOutput{}, errors.New("delete object failed"))

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3DeleteOptions{
						RemoteObjectKey: "path/to/file.txt",
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s3, opt, teardown := tt.setup(t)
			defer teardown()

			err := s3.Delete(context.Background(), opt)
			if tt.wantErr {
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
		wantErr bool
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

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3ListOptions{
						Path: "prefix/",
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
			want:    []string{"prefix/file1.txt", "prefix/file2.txt"},
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

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3ListOptions{
						Path: "prefix/",
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
			want:    []string{"prefix/file1.txt", "prefix/file2.txt", "prefix/file3.txt"},
		},
		{
			name: "error when list objects fails",
			setup: func(t *testing.T) (*S3, S3ListOptions, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.
					On("ListObjects", mock.Anything, mock.Anything).
					Return(&s3.ListObjectsOutput{}, errors.New("list objects failed"))

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3ListOptions{
						Path: "prefix/",
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: true,
			want:    nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s3, opt, teardown := tt.setup(t)
			defer teardown()

			got, err := s3.List(context.Background(), opt)
			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

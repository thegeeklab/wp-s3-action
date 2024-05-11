package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3_types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/thegeeklab/wp-s3-action/aws/mocks"
)

func TestS3Uploader_Upload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (*S3, S3UploadOpt, func())
		wantErr bool
	}{
		{
			name: "skip_upload_when_local_is_empty",
			setup: func(t *testing.T) (*S3, S3UploadOpt, func()) {
				t.Helper()

				return &S3{},
					S3UploadOpt{
						LocalFilePath: "",
					}, func() {}
			},
			wantErr: false,
		},
		{
			name: "error_when_local_file_does_not_exist",
			setup: func(t *testing.T) (*S3, S3UploadOpt, func()) {
				t.Helper()

				return &S3{},
					S3UploadOpt{
						LocalFilePath: "/path/to/non-existent/file",
					}, func() {}
			},
			wantErr: true,
		},
		{
			name: "upload_new_file_with_default_acl_and_content_type",
			setup: func(t *testing.T) (*S3, S3UploadOpt, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{}, &s3_types.NoSuchKey{})
				mockS3Client.On("PutObject", mock.Anything, mock.Anything).Return(&s3.PutObjectOutput{}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3UploadOpt{
						LocalFilePath:   createTempFile(t, "file.txt"),
						RemoteObjectKey: "remote/path/file.txt",
					}, func() {
						mockS3Client.AssertExpectations(t)
					}
			},
			wantErr: false,
		},
		{
			name: "update_metadata_when_content_type_changed",
			setup: func(t *testing.T) (*S3, S3UploadOpt, func()) {
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
					}, S3UploadOpt{
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
			name: "update_metadata_when_acl_changed",
			setup: func(t *testing.T) (*S3, S3UploadOpt, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{
					ETag:        aws.String("'5d41402abc4b2a76b9719d911017c592'"),
					ContentType: aws.String("text/plain; charset=utf-8"),
				}, nil)
				mockS3Client.On("GetObjectAcl", mock.Anything, mock.Anything).Return(&s3.GetObjectAclOutput{
					Grants: []s3_types.Grant{
						{
							Grantee: &s3_types.Grantee{
								URI: aws.String("http://acs.amazonaws.com/groups/global/AllUsers"),
							},
							Permission: s3_types.PermissionWrite,
						},
					},
				}, nil)
				mockS3Client.On("CopyObject", mock.Anything, mock.Anything).Return(&s3.CopyObjectOutput{}, nil)

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
					}, S3UploadOpt{
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
			name: "update_metadata_when_cache_control_changed",
			setup: func(t *testing.T) (*S3, S3UploadOpt, func()) {
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
					}, S3UploadOpt{
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
			name: "update_metadata_when_content_encoding_changed",
			setup: func(t *testing.T) (*S3, S3UploadOpt, func()) {
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
					}, S3UploadOpt{
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
			name: "update_metadata_when_metadata_changed",
			setup: func(t *testing.T) (*S3, S3UploadOpt, func()) {
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
					}, S3UploadOpt{
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
			name: "upload_new_file_when_dry_run_is_true",
			setup: func(t *testing.T) (*S3, S3UploadOpt, func()) {
				t.Helper()

				mockS3Client := mocks.NewMockS3APIClient(t)
				mockS3Client.On("HeadObject", mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{}, &s3_types.NoSuchKey{})

				return &S3{
						client: mockS3Client,
						Bucket: "test-bucket",
						DryRun: true,
					}, S3UploadOpt{
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

			upload, opt, teardown := tt.setup(t)
			defer teardown()

			err := upload.Upload(context.Background(), opt)
			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func createTempFile(t *testing.T, name string) string {
	t.Helper()

	name = filepath.Join(t.TempDir(), name)
	_ = os.WriteFile(name, []byte("hello"), 0o600)

	return name
}

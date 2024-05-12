package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/thegeeklab/wp-s3-action/aws/mocks"
)

var ErrCreateInvalidation = errors.New("create invalidation failed")

func TestCloudfront_Invalidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (*Cloudfront, CloudfrontInvalidateOptions, func())
		wantErr bool
	}{
		{
			name: "invalidate path successfully",
			setup: func(t *testing.T) (*Cloudfront, CloudfrontInvalidateOptions, func()) {
				t.Helper()

				mockClient := mocks.NewMockCloudfrontAPIClient(t)
				mockClient.
					On("CreateInvalidation", mock.Anything, mock.Anything).
					Return(&cloudfront.CreateInvalidationOutput{}, nil)

				return &Cloudfront{
						client:       mockClient,
						Distribution: "test-distribution",
					}, CloudfrontInvalidateOptions{
						Path: "/path/to/invalidate",
					}, func() {
						mockClient.AssertExpectations(t)
					}
			},
			wantErr: false,
		},
		{
			name: "error when create invalidation fails",
			setup: func(t *testing.T) (*Cloudfront, CloudfrontInvalidateOptions, func()) {
				t.Helper()

				mockClient := mocks.NewMockCloudfrontAPIClient(t)
				mockClient.
					On("CreateInvalidation", mock.Anything, mock.Anything).
					Return(&cloudfront.CreateInvalidationOutput{}, ErrCreateInvalidation)

				return &Cloudfront{
						client:       mockClient,
						Distribution: "test-distribution",
					}, CloudfrontInvalidateOptions{
						Path: "/path/to/invalidate",
					}, func() {
						mockClient.AssertExpectations(t)
					}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cf, opt, teardown := tt.setup(t)
			defer teardown()

			err := cf.Invalidate(context.Background(), opt)
			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
		})
	}
}

package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChecksumMode_Set(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    ChecksumMode
		wantErr error
	}{
		{
			name:    "set supported mode",
			value:   "supported",
			want:    ChecksumSupported,
			wantErr: nil,
		},
		{
			name:    "set required mode",
			value:   "required",
			want:    ChecksumRequired,
			wantErr: nil,
		},
		{
			name:    "error on empty mode",
			value:   "",
			want:    "",
			wantErr: ErrInvalidChecksumCalculationMode,
		},
		{
			name:    "error on invalid mode",
			value:   "invalid",
			want:    "",
			wantErr: ErrInvalidChecksumCalculationMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var mode ChecksumMode
			err := mode.Set(tt.value)

			if tt.wantErr != nil {
				assert.ErrorAs(t, err, &tt.wantErr)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, mode)
		})
	}
}

func TestChecksumMode_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode ChecksumMode
		want string
	}{
		{
			name: "string representation of supported mode",
			mode: ChecksumSupported,
			want: "supported",
		},
		{
			name: "string representation of required mode",
			mode: ChecksumRequired,
			want: "required",
		},
		{
			name: "string representation of empty mode",
			mode: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.mode.String())
		})
	}
}

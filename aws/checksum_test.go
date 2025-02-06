package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChecksumMode_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode ChecksumMode
		want bool
	}{
		{
			name: "supported mode is valid",
			mode: ChecksumSupported,
			want: true,
		},
		{
			name: "required mode is valid",
			mode: ChecksumRequired,
			want: true,
		},
		{
			name: "empty mode is invalid",
			mode: "",
			want: false,
		},
		{
			name: "unknown mode is invalid",
			mode: "unknown",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.mode.IsValid())
		})
	}
}

func TestChecksumMode_Set(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    ChecksumMode
		wantErr bool
	}{
		{
			name:    "set supported mode",
			value:   "supported",
			want:    ChecksumSupported,
			wantErr: false,
		},
		{
			name:    "set required mode",
			value:   "required",
			want:    ChecksumRequired,
			wantErr: false,
		},
		{
			name:    "error on empty mode",
			value:   "",
			want:    "",
			wantErr: true,
		},
		{
			name:    "error on invalid mode",
			value:   "invalid",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var mode ChecksumMode
			err := mode.Set(tt.value)

			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidChecksumCalculationMode)

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

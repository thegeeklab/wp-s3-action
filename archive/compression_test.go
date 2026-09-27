package archive

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompressionSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    Compression
		wantErr error
	}{
		{name: "gzip", value: "gzip", want: CompressionGzip},
		{name: "none", value: "none", want: CompressionNone},
		{name: "invalid", value: "zstd", wantErr: ErrInvalidCompression},
		{name: "empty", value: "", wantErr: ErrInvalidCompression},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var c Compression

			err := c.Set(tt.value)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, c)
		})
	}
}

func TestCompressionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode Compression
		want string
	}{
		{name: "gzip", mode: CompressionGzip, want: "gzip"},
		{name: "none", mode: CompressionNone, want: "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.mode.String())
		})
	}
}

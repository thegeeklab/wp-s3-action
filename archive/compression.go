package archive

import (
	"errors"
	"fmt"
)

// Compression identifies the compression algorithm applied to an archive.
type Compression string

const (
	CompressionGzip Compression = "gzip"
	CompressionNone Compression = "none"
)

// ErrInvalidCompression is returned when an unsupported compression value is
// configured.
var ErrInvalidCompression = errors.New("invalid compression")

// Set parses a compression value into the receiver.
func (c *Compression) Set(value string) error {
	switch Compression(value) {
	case CompressionGzip, CompressionNone:
		*c = Compression(value)

		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidCompression, value)
	}
}

// String returns the configured compression value.
func (c Compression) String() string {
	return string(c)
}

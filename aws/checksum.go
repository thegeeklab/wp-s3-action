package aws

import (
	"errors"
	"fmt"
)

// ChecksumMode identifies the checksum calculation mode used for S3 requests.
type ChecksumMode string

const (
	ChecksumSupported ChecksumMode = "supported"
	ChecksumRequired  ChecksumMode = "required"
)

// ErrInvalidChecksumCalculationMode is returned when an unsupported checksum mode is configured.
var ErrInvalidChecksumCalculationMode = errors.New("invalid checksum calculation mode")

// Set parses a checksum mode value into the receiver.
func (cm *ChecksumMode) Set(value string) error {
	switch ChecksumMode(value) {
	case ChecksumSupported, ChecksumRequired:
		*cm = ChecksumMode(value)

		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidChecksumCalculationMode, value)
	}
}

// String returns the configured checksum mode value.
func (cm *ChecksumMode) String() string {
	return string(*cm)
}

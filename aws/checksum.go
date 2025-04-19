package aws

import (
	"errors"
	"fmt"
)

type ChecksumMode string

const (
	ChecksumSupported ChecksumMode = "supported"
	ChecksumRequired  ChecksumMode = "required"
)

var ErrInvalidChecksumCalculationMode = errors.New("invalid checksum calculation mode")

func (cm *ChecksumMode) Set(value string) error {
	switch ChecksumMode(value) {
	case ChecksumSupported, ChecksumRequired:
		*cm = ChecksumMode(value)

		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidChecksumCalculationMode, value)
	}
}

func (cm *ChecksumMode) String() string {
	return string(*cm)
}

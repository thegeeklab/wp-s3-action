package plugin

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/rs/zerolog/log"
	plugin_base "github.com/thegeeklab/wp-plugin-go/v7/plugin"
	"github.com/thegeeklab/wp-s3-action/archive"
	"github.com/thegeeklab/wp-s3-action/aws"
)

// handleArchiveUpload serializes the source directory into a single
// compressed tar object at the target key.
func (p *Plugin) handleArchiveUpload(network plugin_base.Network, s3 S3Runner) error {
	ctx := network.Context

	compression, err := p.archiveCompression()
	if err != nil {
		return err
	}

	if err := p.validateSource(); err != nil {
		return err
	}

	if p.Settings.DryRun {
		log.Debug().Msgf("dry run: skipping archive upload of '%s' to 's3://%s/%s'",
			p.Settings.Source, p.Settings.Bucket, p.Settings.Target)

		return nil
	}

	tmp, err := os.CreateTemp("", "wp-s3-action-archive-*")
	if err != nil {
		return fmt.Errorf("create temp archive: %w", err)
	}

	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	if err := archive.Create(ctx, p.Settings.Source, tmp, compression); err != nil {
		return fmt.Errorf("create archive: %w", err)
	}

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind temp archive: %w", err)
	}

	if err := s3.UploadStream(ctx, aws.S3UploadStreamOptions{
		RemoteObjectKey: p.Settings.Target,
		Body:            tmp,
		ContentType:     archiveContentType(compression),
	}); err != nil {
		return fmt.Errorf("upload archive: %w", err)
	}

	log.Info().Msgf("archive uploaded '%s' to 's3://%s/%s'",
		p.Settings.Source, p.Settings.Bucket, p.Settings.Target)

	return nil
}

// handleArchiveDownload fetches the single archive object at the target key
// and extracts it into the source directory.
func (p *Plugin) handleArchiveDownload(network plugin_base.Network, s3 S3Runner) error {
	ctx := network.Context

	compression, err := p.archiveCompression()
	if err != nil {
		return err
	}

	if p.Settings.DryRun {
		log.Debug().Msgf("dry run: skipping archive download of 's3://%s/%s' to '%s'",
			p.Settings.Bucket, p.Settings.Target, p.Settings.Source)

		return nil
	}

	if err := os.MkdirAll(p.Settings.Source, 0o755); err != nil {
		return fmt.Errorf("create source directory: %w", err)
	}

	tmp, err := os.CreateTemp("", "wp-s3-action-archive-*")
	if err != nil {
		return fmt.Errorf("create temp archive: %w", err)
	}

	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	if err := s3.DownloadStream(ctx, aws.S3DownloadStreamOptions{
		RemoteObjectKey: p.Settings.Target,
	}, tmp); err != nil {
		if errors.Is(err, aws.ErrObjectNotFound) {
			log.Debug().Msgf("archive object 's3://%s/%s' not found, skipping download", p.Settings.Bucket, p.Settings.Target)

			return nil
		}

		return fmt.Errorf("download archive: %w", err)
	}

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind temp archive: %w", err)
	}

	if err := archive.Extract(ctx, p.Settings.Source, tmp, compression); err != nil {
		return fmt.Errorf("extract archive: %w", err)
	}

	log.Info().Msgf("archive extracted 's3://%s/%s' to '%s'",
		p.Settings.Bucket, p.Settings.Target, p.Settings.Source)

	return nil
}

// archiveCompression parses the configured compression algorithm.
func (p *Plugin) archiveCompression() (archive.Compression, error) {
	var compression archive.Compression
	if err := compression.Set(p.Settings.Archive.Compression); err != nil {
		return "", err
	}

	return compression, nil
}

// archiveContentType returns the S3 content type for the given compression.
func archiveContentType(compression archive.Compression) string {
	switch compression {
	case archive.CompressionGzip:
		return "application/gzip"
	default:
		return "application/x-tar"
	}
}

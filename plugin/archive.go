package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	plugin_base "github.com/thegeeklab/wp-plugin-go/v8/plugin"
	plugin_template "github.com/thegeeklab/wp-plugin-go/v8/template"
	"github.com/thegeeklab/wp-s3-action/archive"
	"github.com/thegeeklab/wp-s3-action/aws"
)

// handleArchiveUpload serializes the source directory into a single
// compressed tar object at the target key. The archive is streamed
// directly to S3 via an io.Pipe, avoiding a temp file on disk.
func (p *Plugin) handleArchiveUpload(
	ctx context.Context,
	client http.Client,
	metadata plugin_base.Metadata,
	s3 S3Runner,
) error {
	compression, err := p.archiveCompression()
	if err != nil {
		return err
	}

	if err := p.validateSource(); err != nil {
		return err
	}

	target, err := p.renderTarget(ctx, client, metadata)
	if err != nil {
		return err
	}

	if target == "" {
		return ErrArchiveTargetNotSet
	}

	if p.Settings.DryRun {
		log.Debug().Msgf("dry run: skipping archive upload of '%s' to 's3://%s/%s'",
			p.Settings.Source, p.Settings.Bucket, target)

		return nil
	}

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		if err := archive.Create(ctx, p.Settings.Source, pw, compression); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("create archive: %w", err))
		}
	}()

	if err := s3.UploadStream(ctx, aws.S3UploadStreamOptions{
		RemoteObjectKey: target,
		Body:            pr,
		ContentType:     archiveContentType(compression),
	}); err != nil {
		_ = pr.CloseWithError(fmt.Errorf("upload archive: %w", err))

		return err
	}

	log.Info().Msgf("archive uploaded '%s' to 's3://%s/%s'",
		p.Settings.Source, p.Settings.Bucket, target)

	return nil
}

// handleArchiveDownload fetches the single archive object at the target key
// and extracts it into the source directory. The archive is streamed
// directly from S3 via an io.Pipe, avoiding a temp file on disk.
func (p *Plugin) handleArchiveDownload(
	ctx context.Context,
	client http.Client,
	metadata plugin_base.Metadata,
	s3 S3Runner,
) error {
	compression, err := p.archiveCompression()
	if err != nil {
		return err
	}

	target, err := p.renderTarget(ctx, client, metadata)
	if err != nil {
		return err
	}

	if target == "" {
		return ErrArchiveTargetNotSet
	}

	if p.Settings.DryRun {
		log.Debug().Msgf("dry run: skipping archive download of 's3://%s/%s' to '%s'",
			p.Settings.Bucket, target, p.Settings.Source)

		return nil
	}

	if err := os.MkdirAll(p.Settings.Source, 0o755); err != nil {
		return fmt.Errorf("create source directory: %w", err)
	}

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		if err := s3.DownloadStream(ctx, aws.S3DownloadStreamOptions{
			RemoteObjectKey: target,
		}, pw); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("download archive: %w", err))
		}
	}()

	if err := archive.Extract(ctx, p.Settings.Source, pr, compression); err != nil {
		if errors.Is(err, aws.ErrObjectNotFound) {
			_ = pr.Close()

			log.Debug().Msgf("archive object 's3://%s/%s' not found, skipping download", p.Settings.Bucket, target)

			return nil
		}

		_ = pr.CloseWithError(fmt.Errorf("extract archive: %w", err))

		return err
	}

	log.Info().Msgf("archive extracted 's3://%s/%s' to '%s'",
		p.Settings.Bucket, target, p.Settings.Source)

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

// renderTarget renders the target as a template with Woodpecker metadata.
func (p *Plugin) renderTarget(
	ctx context.Context,
	client http.Client,
	metadata plugin_base.Metadata,
) (string, error) {
	rendered, err := plugin_template.RenderTrim(
		ctx,
		client,
		p.Settings.Target,
		metadata,
	)
	if err != nil {
		return "", fmt.Errorf("render target template: %w", err)
	}

	// A rendered metadata value can begin with a slash. Normalize it the same
	// way Validate normalizes the raw target so archive and non-archive modes
	// produce consistent keys.
	return strings.TrimPrefix(rendered, "/"), nil
}

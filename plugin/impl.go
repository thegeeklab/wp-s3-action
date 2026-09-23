package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	plugin_base "github.com/thegeeklab/wp-plugin-go/v7/plugin"
	"github.com/thegeeklab/wp-s3-action/aws"
)

var (
	ErrEmptySourceDirectory   = errors.New("source directory is empty")
	ErrActionUnknown          = errors.New("action not found")
	ErrPathTraversal          = errors.New("refusing path with traversal outside source directory")
	ErrRedirectsNotSet        = errors.New("redirects setting is required for the redirect action")
	ErrCloudFrontDistribution = errors.New(
		"cloudfront distribution is required for invalidate-cloudfront action",
	)
	ErrTargetNotSet   = errors.New("target is required for the delete action")
	ErrDownloadTarget = errors.New(
		"target is required for the download action to avoid pulling the entire bucket",
	)
	ErrInvalidMaxConcurrency = errors.New("max-concurrency must be at least 1")
)

const (
	S3ActionUpload               S3Action = "upload"
	S3ActionDownload             S3Action = "download"
	S3ActionDelete               S3Action = "delete"
	S3ActionRedirect             S3Action = "redirect"
	S3ActionInvalidateCloudFront S3Action = "invalidate-cloudfront"
)

// Execute provides the implementation of the plugin.
func (p *Plugin) run(ctx context.Context) error {
	if err := p.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := p.Execute(); err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	return nil
}

// Validate handles the settings validation of the plugin.
func (p *Plugin) Validate() error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("error while retrieving working directory: %w", err)
	}

	p.Settings.Source = filepath.Join(wd, p.Settings.Source)
	p.Settings.Target = strings.TrimPrefix(p.Settings.Target, "/")

	if p.Settings.MaxConcurrency < 1 {
		return ErrInvalidMaxConcurrency
	}

	p.Settings.Action = make([]S3Action, 0, len(p.Settings.ActionStrings))

	for _, actionStr := range p.Settings.ActionStrings {
		action := S3Action(actionStr)

		switch action {
		case S3ActionUpload:
			// no extra dependencies
		case S3ActionDownload:
			if p.Settings.Target == "" {
				return ErrDownloadTarget
			}
		case S3ActionDelete:
			if p.Settings.Target == "" {
				return ErrTargetNotSet
			}
		case S3ActionRedirect:
			if len(p.Settings.Redirects) == 0 {
				return ErrRedirectsNotSet
			}
		case S3ActionInvalidateCloudFront:
			if p.Settings.CloudFront.Distribution == "" {
				return ErrCloudFrontDistribution
			}
		default:
			return fmt.Errorf("%w: %s", ErrActionUnknown, actionStr)
		}

		p.Settings.Action = append(p.Settings.Action, action)
	}

	return nil
}

// Execute provides the implementation of the plugin.
func (p *Plugin) Execute() error {
	network, err := p.GetNetwork()
	if err != nil {
		return fmt.Errorf("error while getting network configuration: %w", err)
	}

	client, err := aws.NewClient(
		network.Context,
		p.Settings.Endpoint,
		p.Settings.Region,
		p.Settings.AccessKey,
		p.Settings.SecretKey,
		p.Settings.PathStyle,
		p.Settings.ChecksumCalculation,
		network.Client,
	)
	if err != nil {
		return fmt.Errorf("error while creating AWS client: %w", err)
	}

	client.S3.Bucket = p.Settings.Bucket
	client.S3.DryRun = p.Settings.DryRun

	for _, action := range p.Settings.Action {
		switch action {
		case S3ActionUpload:
			if err := p.handleUpload(network, client); err != nil {
				return err
			}
		case S3ActionDownload:
			if err := p.handleDownload(network, client); err != nil {
				return err
			}
		case S3ActionDelete:
			if err := p.handleDelete(network, client); err != nil {
				return err
			}
		case S3ActionRedirect:
			if err := p.handleRedirect(network, client); err != nil {
				return err
			}
		case S3ActionInvalidateCloudFront:
			if err := p.handleInvalidateCloudFront(network, client); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Plugin) handleUpload(network plugin_base.Network, client *aws.Client) error {
	return p.runActionJobs(network, client, S3ActionUpload, func(jobs chan<- Job) error {
		if err := p.validateSource(); err != nil {
			return err
		}

		yield := jobChannelYield(network.Context, jobs)

		local, err := p.createUploadJobs(yield)
		if err != nil {
			return err
		}

		if p.Settings.Upload.Delete {
			if err := p.createMirrorDeleteJobs(network.Context, client, local, yield); err != nil {
				return err
			}
		}

		return nil
	})
}

func (p *Plugin) handleDownload(network plugin_base.Network, client *aws.Client) error {
	if err := os.MkdirAll(p.Settings.Source, 0o755); err != nil {
		return fmt.Errorf("create source directory: %w", err)
	}

	return p.runActionJobs(network, client, S3ActionDownload, func(jobs chan<- Job) error {
		yield := jobChannelYield(network.Context, jobs)

		return client.S3.List(network.Context, aws.S3ListOptions{Path: p.Settings.Target}, func(remoteKey string) error {
			if !withinTarget(remoteKey, p.Settings.Target) {
				return nil
			}

			localPath, err := remoteKeyToLocalPath(remoteKey, p.Settings.Target, p.Settings.Source)
			if err != nil {
				return err
			}

			return yield(Job{local: localPath, remote: remoteKey, action: S3ActionDownload})
		})
	})
}

func (p *Plugin) handleDelete(network plugin_base.Network, client *aws.Client) error {
	return p.runActionJobs(network, client, S3ActionDelete, func(jobs chan<- Job) error {
		yield := jobChannelYield(network.Context, jobs)
		batch := make([]string, 0, aws.MaxDeleteBatch)

		emit := func() error {
			return emitDeleteBatch(&batch, yield)
		}

		err := client.S3.List(network.Context, aws.S3ListOptions{Path: p.Settings.Target}, func(remoteKey string) error {
			if !withinTarget(remoteKey, p.Settings.Target) {
				return nil
			}

			batch = append(batch, remoteKey)
			if len(batch) >= aws.MaxDeleteBatch {
				return emit()
			}

			return nil
		})
		if err != nil {
			return err
		}

		return emit()
	})
}

func (p *Plugin) handleRedirect(network plugin_base.Network, client *aws.Client) error {
	return p.runActionJobs(network, client, S3ActionRedirect, func(jobs chan<- Job) error {
		yield := jobChannelYield(network.Context, jobs)

		for path, location := range p.Settings.Redirects {
			path = strings.TrimPrefix(path, "/")

			err := yield(Job{
				local:  filepath.Join(p.Settings.Target, path),
				remote: location,
				action: S3ActionRedirect,
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// runActionJobs wires the build callback to a streaming job channel and
// dispatches the produced jobs with bounded concurrency. The build
// callback closes the channel when it has finished emitting jobs.
func (p *Plugin) runActionJobs(
	network plugin_base.Network,
	client *aws.Client,
	action S3Action,
	build func(chan<- Job) error,
) error {
	jobs := make(chan Job)
	buildErr := make(chan error, 1)

	go func() {
		defer close(jobs)

		buildErr <- build(jobs)
	}()

	if err := p.runJobs(network.Context, client, jobs); err != nil {
		<-buildErr

		return fmt.Errorf("error while running %s jobs: %w", action, err)
	}

	if err := <-buildErr; err != nil {
		return fmt.Errorf("error while building %s jobs: %w", action, err)
	}

	return nil
}

func (p *Plugin) handleInvalidateCloudFront(network plugin_base.Network, client *aws.Client) error {
	if p.Settings.DryRun {
		log.Debug().Msgf("dry run: skipping cloudfront invalidation of '/%s/*'", p.Settings.Target)

		return nil
	}

	client.Cloudfront.Distribution = p.Settings.CloudFront.Distribution

	if err := client.Cloudfront.Invalidate(network.Context, aws.CloudfrontInvalidateOptions{
		Path: path.Join("/", p.Settings.Target, "*"),
	}); err != nil {
		return fmt.Errorf("error while invalidating cloudfront distribution: %w", err)
	}

	return nil
}

func (p *Plugin) validateSource() error {
	entries, err := os.ReadDir(p.Settings.Source)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	if len(entries) == 0 {
		if !p.Settings.Upload.AllowEmptySource {
			return fmt.Errorf("%w: %s", ErrEmptySourceDirectory, p.Settings.Source)
		}

		log.Warn().Msgf("%s: %s", ErrEmptySourceDirectory, p.Settings.Source)
	}

	return nil
}

func (p *Plugin) createUploadJobs(yield func(Job) error) ([]string, error) {
	local := make([]string, 0)

	err := filepath.Walk(p.Settings.Source, func(filePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		localPath, relErr := filepath.Rel(p.Settings.Source, filePath)
		if relErr != nil {
			return relErr
		}

		local = append(local, localPath)

		return yield(Job{
			local:  filepath.Join(p.Settings.Source, localPath),
			remote: filepath.Join(p.Settings.Target, localPath),
			action: S3ActionUpload,
		})
	})

	return local, err
}

func (p *Plugin) createMirrorDeleteJobs(
	ctx context.Context,
	client *aws.Client,
	local []string,
	yield func(Job) error,
) error {
	for path := range p.Settings.Redirects {
		local = append(local, strings.TrimPrefix(path, "/"))
	}

	localSet := make(map[string]struct{}, len(local))
	for _, l := range local {
		localSet[l] = struct{}{}
	}

	batch := make([]string, 0, aws.MaxDeleteBatch)

	emit := func() error {
		return emitDeleteBatch(&batch, yield)
	}

	err := client.S3.List(ctx, aws.S3ListOptions{Path: p.Settings.Target}, func(remoteKey string) error {
		if !withinTarget(remoteKey, p.Settings.Target) {
			return nil
		}

		if _, ok := localSet[stripTargetPrefix(remoteKey, p.Settings.Target)]; ok {
			return nil
		}

		batch = append(batch, remoteKey)
		if len(batch) >= aws.MaxDeleteBatch {
			return emit()
		}

		return nil
	})
	if err != nil {
		return err
	}

	return emit()
}

func (p *Plugin) runJobs(ctx context.Context, client *aws.Client, jobs <-chan Job) error {
	jobSem := make(chan struct{}, p.Settings.MaxConcurrency)

	log.Info().Msgf("Processing bucket '%s'", p.Settings.Bucket)

	var wg sync.WaitGroup

	firstErr := make(chan error, 1)
	setErr := func(err error) {
		select {
		case firstErr <- err:
		default:
		}
	}

	for job := range jobs {
		select {
		case <-ctx.Done():
			setErr(ctx.Err())
			wg.Wait()

			return fmt.Errorf("job failed: %w", ctx.Err())
		case jobSem <- struct{}{}:
		}

		wg.Add(1)

		go func(job Job) {
			defer wg.Done()
			defer func() { <-jobSem }()

			if err := job.execute(ctx, client, p.Settings); err != nil {
				setErr(err)
			}
		}(job)
	}

	wg.Wait()

	select {
	case err := <-firstErr:
		return fmt.Errorf("job failed: %w", err)
	default:
		return nil
	}
}

func (j Job) execute(ctx context.Context, client *aws.Client, settings *Settings) error {
	switch j.action {
	case S3ActionUpload:
		return client.S3.Upload(ctx, aws.S3UploadOptions{
			LocalFilePath:   j.local,
			RemoteObjectKey: j.remote,
			ACL:             settings.Upload.ACL,
			ContentType:     settings.Upload.ContentType,
			ContentEncoding: settings.Upload.ContentEncoding,
			CacheControl:    settings.Upload.CacheControl,
			Metadata:        settings.Upload.Metadata,
		})
	case S3ActionDownload:
		return client.S3.Download(ctx, aws.S3DownloadOptions{
			LocalRoot:       settings.Source,
			LocalFilePath:   j.local,
			RemoteObjectKey: j.remote,
		})
	case S3ActionRedirect:
		return client.S3.Redirect(ctx, aws.S3RedirectOptions{
			Path:     j.local,
			Location: j.remote,
		})
	case S3ActionDelete:
		return client.S3.Delete(ctx, aws.S3DeleteOptions{RemoteObjectKeys: j.remoteSet})
	default:
		return fmt.Errorf("%w: %s", ErrActionUnknown, j.action)
	}
}

func remoteKeyToLocalPath(remoteKey, target, source string) (string, error) {
	cleanSource := filepath.Clean(source)
	localPath := filepath.Join(source, stripTargetPrefix(remoteKey, target))

	cleanLocal := filepath.Clean(localPath)
	if cleanLocal != cleanSource && !strings.HasPrefix(cleanLocal, cleanSource+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s", ErrPathTraversal, remoteKey)
	}

	if !pathWithinRoot(cleanSource, cleanLocal) {
		return "", fmt.Errorf("%w: %s", ErrPathTraversal, remoteKey)
	}

	return localPath, nil
}

// stripTargetPrefix removes the target prefix from key, treating target as a
// slash-delimited S3 path boundary. A trailing slash on target is ignored so
// "foo" and "foo/" behave identically.
func stripTargetPrefix(key, target string) string {
	target = strings.TrimSuffix(target, "/")
	if target == "" {
		return key
	}

	return strings.TrimPrefix(key, target+"/")
}

// emitDeleteBatch swaps the current batch into a Job whose action is
// delete and forwards it via yield. The batch pointer is reset to a
// fresh empty slice so the caller can keep appending. A no-op when the
// batch is empty.
func emitDeleteBatch(batch *[]string, yield func(Job) error) error {
	if len(*batch) == 0 {
		return nil
	}

	chunk := *batch
	*batch = make([]string, 0, aws.MaxDeleteBatch)

	return yield(Job{remoteSet: chunk, action: S3ActionDelete})
}

// jobChannelYield returns a yield function that forwards each Job to
// jobs while honoring ctx cancellation so a stuck consumer cannot
// deadlock the producer.
func jobChannelYield(ctx context.Context, jobs chan<- Job) func(Job) error {
	return func(j Job) error {
		select {
		case jobs <- j:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// withinTarget reports whether key is the target itself or nested under it.
func withinTarget(key, target string) bool {
	target = strings.TrimSuffix(target, "/")
	if target == "" {
		return true
	}

	return key == target || strings.HasPrefix(key, target+"/")
}

// pathWithinRoot reports whether path stays within root. It resolves symlinks
// at every existing ancestor of path; the deepest existing ancestor determines
// the comparison. If neither root nor any ancestor of path resolves, the
// conservative default is false (path cannot be guaranteed to be contained).
func pathWithinRoot(root, path string) bool {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}

	current := path
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return resolved == resolvedRoot ||
				strings.HasPrefix(resolved, resolvedRoot+string(filepath.Separator))
		}

		parent := filepath.Dir(current)
		if parent == current {
			return false
		}

		current = parent
	}
}

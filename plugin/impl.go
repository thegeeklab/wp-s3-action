package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

var ErrTypeAssertionFailed = errors.New("type assertion failed")

// Execute provides the implementation of the plugin.
//
//nolint:revive
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

	return nil
}

// Execute provides the implementation of the plugin.
func (p *Plugin) Execute() error {
	p.Settings.Jobs = make([]Job, 1)
	p.Settings.Client = NewAWS(p)

	if err := p.createSyncJobs(); err != nil {
		return fmt.Errorf("error while creating sync job: %w", err)
	}

	if len(p.Settings.CloudFrontDistribution) > 0 {
		p.Settings.Jobs = append(p.Settings.Jobs, Job{
			local:  "",
			remote: filepath.Join("/", p.Settings.Target, "*"),
			action: "invalidateCloudFront",
		})
	}

	if err := p.runJobs(); err != nil {
		return fmt.Errorf("error while creating sync job: %w", err)
	}

	return nil
}

func (p *Plugin) createSyncJobs() error {
	remote, err := p.Settings.Client.List(p.Settings.Target)
	if err != nil {
		return err
	}

	local := make([]string, 0)

	err = filepath.Walk(p.Settings.Source, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		localPath := path
		if p.Settings.Source != "." {
			localPath = strings.TrimPrefix(path, p.Settings.Source)
			localPath = strings.TrimPrefix(localPath, "/")
		}

		local = append(local, localPath)

		p.Settings.Jobs = append(p.Settings.Jobs, Job{
			local:  filepath.Join(p.Settings.Source, localPath),
			remote: filepath.Join(p.Settings.Target, localPath),
			action: "upload",
		})

		return nil
	})
	if err != nil {
		return err
	}

	for path, location := range p.Settings.Redirects {
		path = strings.TrimPrefix(path, "/")
		local = append(local, path)
		p.Settings.Jobs = append(p.Settings.Jobs, Job{
			local:  path,
			remote: location,
			action: "redirect",
		})
	}

	if p.Settings.Delete {
		for _, remote := range remote {
			found := false
			remotePath := strings.TrimPrefix(remote, p.Settings.Target+"/")

			for _, l := range local {
				if l == remotePath {
					found = true

					break
				}
			}

			if !found {
				p.Settings.Jobs = append(p.Settings.Jobs, Job{
					local:  "",
					remote: remote,
					action: "delete",
				})
			}
		}
	}

	return nil
}

func (p *Plugin) runJobs() error {
	client := p.Settings.Client
	jobChan := make(chan struct{}, p.Settings.MaxConcurrency)
	results := make(chan *Result, len(p.Settings.Jobs))

	var invalidateJob *Job

	log.Info().Msgf("Synchronizing with bucket '%s'", p.Settings.Bucket)

	for _, job := range p.Settings.Jobs {
		jobChan <- struct{}{}

		go func(job Job) {
			var err error

			switch job.action {
			case "upload":
				err = client.Upload(job.local, job.remote)
			case "redirect":
				err = client.Redirect(job.local, job.remote)
			case "delete":
				err = client.Delete(job.remote)
			case "invalidateCloudFront":
				invalidateJob = &job
			default:
				err = nil
			}
			results <- &Result{job, err}

			<-jobChan
		}(job)
	}

	for range p.Settings.Jobs {
		r := <-results
		if r.err != nil {
			return fmt.Errorf("failed to %s %s to %s: %w", r.j.action, r.j.local, r.j.remote, r.err)
		}
	}

	if invalidateJob != nil {
		err := client.Invalidate(invalidateJob.remote)
		if err != nil {
			return fmt.Errorf("failed to %s %s to %s: %w", invalidateJob.action, invalidateJob.local, invalidateJob.remote, err)
		}
	}

	return nil
}

package plugin

import (
	"context"
	"crypto/md5" //nolint:gosec
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cf_types "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3_types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/rs/zerolog/log"
	"github.com/ryanuber/go-glob"
)

type AWS struct {
	client   *s3.Client
	cfClient *cloudfront.Client
	remote   []string
	local    []string
	plugin   *Plugin
}

func NewAWS(plugin *Plugin) (AWS, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(plugin.Settings.Region),
	)
	if err != nil {
		return AWS{}, fmt.Errorf("error while loading AWS config: %w", err)
	}

	if plugin.Settings.Endpoint != "" {
		cfg.BaseEndpoint = aws.String(plugin.Settings.Endpoint)
	}

	// allowing to use the instance role or provide a key and secret
	if plugin.Settings.AccessKey != "" && plugin.Settings.SecretKey != "" {
		cfg.Credentials = credentials.NewStaticCredentialsProvider(plugin.Settings.AccessKey, plugin.Settings.SecretKey, "")
	}

	c := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = plugin.Settings.PathStyle
	})
	cf := cloudfront.NewFromConfig(cfg)
	r := make([]string, 1)
	l := make([]string, 1)

	return AWS{c, cf, r, l, plugin}, nil
}

//nolint:gocognit,gocyclo,maintidx
func (a *AWS) Upload(local, remote string) error {
	plugin := a.plugin

	if local == "" {
		return nil
	}

	file, err := os.Open(local)
	if err != nil {
		return err
	}

	defer file.Close()

	var acl string

	for pattern := range plugin.Settings.ACL {
		if match := glob.Glob(pattern, local); match {
			acl = plugin.Settings.ACL[pattern]

			break
		}
	}

	if acl == "" {
		acl = "private"
	}

	fileExt := filepath.Ext(local)

	var contentType string

	for patternExt := range plugin.Settings.ContentType {
		if patternExt == fileExt {
			contentType = plugin.Settings.ContentType[patternExt]

			break
		}
	}

	if contentType == "" {
		contentType = mime.TypeByExtension(fileExt)
	}

	var contentEncoding string

	for patternExt := range plugin.Settings.ContentEncoding {
		if patternExt == fileExt {
			contentEncoding = plugin.Settings.ContentEncoding[patternExt]

			break
		}
	}

	var cacheControl string

	for pattern := range plugin.Settings.CacheControl {
		if match := glob.Glob(pattern, local); match {
			cacheControl = plugin.Settings.CacheControl[pattern]

			break
		}
	}

	metadata := map[string]string{}

	for pattern := range plugin.Settings.Metadata {
		if match := glob.Glob(pattern, local); match {
			for k, v := range plugin.Settings.Metadata[pattern] {
				metadata[k] = v
			}

			break
		}
	}

	var noSuchKeyErr *s3_types.NoSuchKey

	head, err := a.client.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: aws.String(plugin.Settings.Bucket),
		Key:    aws.String(remote),
	})
	if err != nil {
		if errors.As(err, &noSuchKeyErr) {
			return err
		}

		log.Debug().Msgf(
			"'%s' not found in bucket, uploading with content-type '%s' and permissions '%s'",
			local,
			contentType,
			acl,
		)

		putObject := &s3.PutObjectInput{
			Bucket:      aws.String(plugin.Settings.Bucket),
			Key:         aws.String(remote),
			Body:        file,
			ContentType: aws.String(contentType),
			ACL:         s3_types.ObjectCannedACL(acl),
			Metadata:    metadata,
		}

		if len(cacheControl) > 0 {
			putObject.CacheControl = aws.String(cacheControl)
		}

		if len(contentEncoding) > 0 {
			putObject.ContentEncoding = aws.String(contentEncoding)
		}

		// skip upload during dry run
		if a.plugin.Settings.DryRun {
			return nil
		}

		_, err = a.client.PutObject(context.TODO(), putObject)

		return err
	}

	//nolint:gosec
	hash := md5.New()
	_, _ = io.Copy(hash, file)
	sum := fmt.Sprintf("'%x'", hash.Sum(nil))

	//nolint:nestif
	if sum == *head.ETag {
		shouldCopy := false

		if head.ContentType == nil && contentType != "" {
			log.Debug().Msgf("content-type has changed from unset to %s", contentType)

			shouldCopy = true
		}

		if !shouldCopy && head.ContentType != nil && contentType != *head.ContentType {
			log.Debug().Msgf("content-type has changed from %s to %s", *head.ContentType, contentType)

			shouldCopy = true
		}

		if !shouldCopy && head.ContentEncoding == nil && contentEncoding != "" {
			log.Debug().Msgf("Content-Encoding has changed from unset to %s", contentEncoding)

			shouldCopy = true
		}

		if !shouldCopy && head.ContentEncoding != nil && contentEncoding != *head.ContentEncoding {
			log.Debug().Msgf("Content-Encoding has changed from %s to %s", *head.ContentEncoding, contentEncoding)

			shouldCopy = true
		}

		if !shouldCopy && head.CacheControl == nil && cacheControl != "" {
			log.Debug().Msgf("cache-control has changed from unset to %s", cacheControl)

			shouldCopy = true
		}

		if !shouldCopy && head.CacheControl != nil && cacheControl != *head.CacheControl {
			log.Debug().Msgf("cache-control has changed from %s to %s", *head.CacheControl, cacheControl)

			shouldCopy = true
		}

		if !shouldCopy && len(head.Metadata) != len(metadata) {
			log.Debug().Msgf("count of metadata values has changed for %s", local)

			shouldCopy = true
		}

		if !shouldCopy && len(metadata) > 0 {
			for k, v := range metadata {
				if hv, ok := head.Metadata[k]; ok {
					if v != hv {
						log.Debug().Msgf("metadata values have changed for %s", local)

						shouldCopy = true

						break
					}
				}
			}
		}

		if !shouldCopy {
			grant, err := a.client.GetObjectAcl(context.TODO(), &s3.GetObjectAclInput{
				Bucket: aws.String(plugin.Settings.Bucket),
				Key:    aws.String(remote),
			})
			if err != nil {
				return err
			}

			previousACL := "private"

			for _, grant := range grant.Grants {
				grantee := *grant.Grantee
				if grantee.URI != nil {
					if *grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
						if grant.Permission == "READ" {
							previousACL = "public-read"
						} else if grant.Permission == "WRITE" {
							previousACL = "public-read-write"
						}
					}

					if *grantee.URI == "http://acs.amazonaws.com/groups/global/AuthenticatedUsers" {
						if grant.Permission == "READ" {
							previousACL = "authenticated-read"
						}
					}
				}
			}

			if previousACL != acl {
				log.Debug().Msgf("permissions for '%s' have changed from '%s' to '%s'", remote, previousACL, acl)

				shouldCopy = true
			}
		}

		if !shouldCopy {
			log.Debug().Msgf("skipping '%s' because hashes and metadata match", local)

			return nil
		}

		log.Debug().Msgf("updating metadata for '%s' content-type: '%s', ACL: '%s'", local, contentType, acl)

		copyObject := &s3.CopyObjectInput{
			Bucket:            aws.String(plugin.Settings.Bucket),
			Key:               aws.String(remote),
			CopySource:        aws.String(fmt.Sprintf("%s/%s", plugin.Settings.Bucket, remote)),
			ACL:               s3_types.ObjectCannedACL(acl),
			ContentType:       aws.String(contentType),
			Metadata:          metadata,
			MetadataDirective: s3_types.MetadataDirectiveReplace,
		}

		if len(cacheControl) > 0 {
			copyObject.CacheControl = aws.String(cacheControl)
		}

		if len(contentEncoding) > 0 {
			copyObject.ContentEncoding = aws.String(contentEncoding)
		}

		// skip update if dry run
		if a.plugin.Settings.DryRun {
			return nil
		}

		_, err = a.client.CopyObject(context.Background(), copyObject)

		return err
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		return err
	}

	log.Debug().Msgf("uploading '%s' with content-type '%s' and permissions '%s'", local, contentType, acl)

	putObject := &s3.PutObjectInput{
		Bucket:      aws.String(plugin.Settings.Bucket),
		Key:         aws.String(remote),
		Body:        file,
		ContentType: aws.String(contentType),
		ACL:         s3_types.ObjectCannedACL(acl),
		Metadata:    metadata,
	}

	if len(cacheControl) > 0 {
		putObject.CacheControl = aws.String(cacheControl)
	}

	if len(contentEncoding) > 0 {
		putObject.ContentEncoding = aws.String(contentEncoding)
	}

	// skip upload if dry run
	if a.plugin.Settings.DryRun {
		return nil
	}

	_, err = a.client.PutObject(context.TODO(), putObject)

	return err
}

func (a *AWS) Redirect(path, location string) error {
	plugin := a.plugin

	log.Debug().Msgf("adding redirect from '%s' to '%s'", path, location)

	if a.plugin.Settings.DryRun {
		return nil
	}

	_, err := a.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:                  aws.String(plugin.Settings.Bucket),
		Key:                     aws.String(path),
		ACL:                     s3_types.ObjectCannedACLPublicRead,
		WebsiteRedirectLocation: aws.String(location),
	})

	return err
}

func (a *AWS) Delete(remote string) error {
	plugin := a.plugin

	log.Debug().Msgf("removing remote file '%s'", remote)

	if a.plugin.Settings.DryRun {
		return nil
	}

	_, err := a.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(plugin.Settings.Bucket),
		Key:    aws.String(remote),
	})

	return err
}

func (a *AWS) List(path string) ([]string, error) {
	plugin := a.plugin

	remote := make([]string, 0)

	resp, err := a.client.ListObjects(context.TODO(), &s3.ListObjectsInput{
		Bucket: aws.String(plugin.Settings.Bucket),
		Prefix: aws.String(path),
	})
	if err != nil {
		return remote, err
	}

	for _, item := range resp.Contents {
		remote = append(remote, *item.Key)
	}

	for *resp.IsTruncated {
		resp, err = a.client.ListObjects(context.TODO(), &s3.ListObjectsInput{
			Bucket: aws.String(plugin.Settings.Bucket),
			Prefix: aws.String(path),
			Marker: aws.String(remote[len(remote)-1]),
		})
		if err != nil {
			return remote, err
		}

		for _, item := range resp.Contents {
			remote = append(remote, *item.Key)
		}
	}

	return remote, nil
}

func (a *AWS) Invalidate(invalidatePath string) error {
	p := a.plugin

	log.Debug().Msgf("invalidating '%s'", invalidatePath)

	_, err := a.cfClient.CreateInvalidation(context.TODO(), &cloudfront.CreateInvalidationInput{
		DistributionId: aws.String(p.Settings.CloudFrontDistribution),
		InvalidationBatch: &cf_types.InvalidationBatch{
			CallerReference: aws.String(time.Now().Format(time.RFC3339Nano)),
			Paths: &cf_types.Paths{
				Quantity: aws.Int32(1),
				Items: []string{
					invalidatePath,
				},
			},
		},
	})

	return err
}

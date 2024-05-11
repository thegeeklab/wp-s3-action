package aws

import (
	"context"
	"crypto/md5" //nolint:gosec
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3_types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/rs/zerolog/log"
)

type Client struct {
	client     APIClient
	cloudfront *cloudfront.Client
}

type S3Uploader struct {
	client APIClient
	Opt    S3UploaderOpt
}

type S3UploaderOpt struct {
	Local           string
	Remote          string
	ACL             map[string]string
	ContentType     map[string]string
	ContentEncoding map[string]string
	CacheControl    map[string]string
	Metadata        map[string]map[string]string
	Bucket          string
	DryRun          bool
}

func NewClient(ctx context.Context, url, region, accessKey, secretKey string, pathStyle bool) (*Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("error while loading AWS config: %w", err)
	}

	if url != "" {
		cfg.BaseEndpoint = aws.String(url)
	}

	// allowing to use the instance role or provide a key and secret
	if accessKey != "" && secretKey != "" {
		cfg.Credentials = credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
	}

	c := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = pathStyle
	})
	cf := cloudfront.NewFromConfig(cfg)

	return &Client{
		client:     c,
		cloudfront: cf,
	}, nil
}

// Upload uploads a file to an S3 bucket with the specified options.
// It first checks if the file already exists in the bucket and compares the metadata.
// If the metadata has changed, it updates the file in the bucket.
// Otherwise, it skips the upload.
// If the file does not exist in the bucket, it uploads the file with the specified metadata.
// The function returns an error if the upload or update fails.
//
//nolint:gocognit,gocyclo,maintidx
//nolint:gocognit,gocyclo,maintidx
func (u *S3Uploader) Upload() error {
	if u.Opt.Local == "" {
		return nil
	}

	file, err := os.Open(u.Opt.Local)
	if err != nil {
		return err
	}

	defer file.Close()

	var acl string

	for pattern := range u.Opt.ACL {
		if match, _ := filepath.Match(pattern, u.Opt.Local); match {
			acl = u.Opt.ACL[pattern]

			break
		}
	}

	if acl == "" {
		acl = "private"
	}

	fileExt := filepath.Ext(u.Opt.Local)

	var contentType string

	for patternExt := range u.Opt.ContentType {
		if patternExt == fileExt {
			contentType = u.Opt.ContentType[patternExt]

			break
		}
	}

	if contentType == "" {
		contentType = mime.TypeByExtension(fileExt)
	}

	var contentEncoding string

	for patternExt := range u.Opt.ContentEncoding {
		if patternExt == fileExt {
			contentEncoding = u.Opt.ContentEncoding[patternExt]

			break
		}
	}

	var cacheControl string

	for pattern := range u.Opt.CacheControl {
		if match, _ := filepath.Match(pattern, u.Opt.Local); match {
			cacheControl = u.Opt.CacheControl[pattern]

			break
		}
	}

	metadata := map[string]string{}

	for pattern := range u.Opt.Metadata {
		if match, _ := filepath.Match(pattern, u.Opt.Local); match {
			for k, v := range u.Opt.Metadata[pattern] {
				metadata[k] = v
			}

			break
		}
	}

	var noSuchKeyErr *s3_types.NoSuchKey

	head, err := u.client.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: aws.String(u.Opt.Bucket),
		Key:    aws.String(u.Opt.Remote),
	})
	if err != nil {
		if !errors.As(err, &noSuchKeyErr) {
			return err
		}

		log.Debug().Msgf(
			"'%s' not found in bucket, uploading with content-type '%s' and permissions '%s'",
			u.Opt.Local,
			contentType,
			acl,
		)

		putObject := &s3.PutObjectInput{
			Bucket:      aws.String(u.Opt.Bucket),
			Key:         aws.String(u.Opt.Remote),
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
		if u.Opt.DryRun {
			return nil
		}

		_, err = u.client.PutObject(context.TODO(), putObject)

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
			log.Debug().Msgf("count of metadata values has changed for %s", u.Opt.Local)

			shouldCopy = true
		}

		if !shouldCopy && len(metadata) > 0 {
			for k, v := range metadata {
				if hv, ok := head.Metadata[k]; ok {
					if v != hv {
						log.Debug().Msgf("metadata values have changed for %s", u.Opt.Local)

						shouldCopy = true

						break
					}
				}
			}
		}

		if !shouldCopy {
			grant, err := u.client.GetObjectAcl(context.TODO(), &s3.GetObjectAclInput{
				Bucket: aws.String(u.Opt.Bucket),
				Key:    aws.String(u.Opt.Remote),
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
				log.Debug().Msgf("permissions for '%s' have changed from '%s' to '%s'", u.Opt.Remote, previousACL, acl)

				shouldCopy = true
			}
		}

		if !shouldCopy {
			log.Debug().Msgf("skipping '%s' because hashes and metadata match", u.Opt.Local)

			return nil
		}

		log.Debug().Msgf("updating metadata for '%s' content-type: '%s', ACL: '%s'", u.Opt.Local, contentType, acl)

		copyObject := &s3.CopyObjectInput{
			Bucket:            aws.String(u.Opt.Bucket),
			Key:               aws.String(u.Opt.Remote),
			CopySource:        aws.String(fmt.Sprintf("%s/%s", u.Opt.Bucket, u.Opt.Remote)),
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
		if u.Opt.DryRun {
			return nil
		}

		_, err = u.client.CopyObject(context.Background(), copyObject)

		return err
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		return err
	}

	log.Debug().Msgf("uploading '%s' with content-type '%s' and permissions '%s'", u.Opt.Local, contentType, acl)

	putObject := &s3.PutObjectInput{
		Bucket:      aws.String(u.Opt.Bucket),
		Key:         aws.String(u.Opt.Remote),
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
	if u.Opt.DryRun {
		return nil
	}

	_, err = u.client.PutObject(context.TODO(), putObject)

	return err
}

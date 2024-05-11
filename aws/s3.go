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
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
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

// Upload uploads a file to an S3 bucket. It first checks if the file already exists in the
// bucket and if the content and metadata have not changed. If the file has not changed,
// it skips the upload and just updates the metadata. If the file has changed, it uploads
// the new file to the bucket.
func (u *S3Uploader) Upload() error {
	if u.Opt.Local == "" {
		return nil
	}

	file, err := os.Open(u.Opt.Local)
	if err != nil {
		return err
	}
	defer file.Close()

	acl := getACL(u.Opt.Local, u.Opt.ACL)
	contentType := getContentType(u.Opt.Local, u.Opt.ContentType)
	contentEncoding := getContentEncoding(u.Opt.Local, u.Opt.ContentEncoding)
	cacheControl := getCacheControl(u.Opt.Local, u.Opt.CacheControl)
	metadata := getMetadata(u.Opt.Local, u.Opt.Metadata)

	head, err := u.client.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: &u.Opt.Bucket,
		Key:    &u.Opt.Remote,
	})
	if err != nil {
		var noSuchKeyError *types.NoSuchKey
		if !errors.As(err, &noSuchKeyError) {
			return err
		}

		log.Debug().Msgf(
			"'%s' not found in bucket, uploading with content-type '%s' and permissions '%s'",
			u.Opt.Local,
			contentType,
			acl,
		)

		if u.Opt.DryRun {
			return nil
		}

		_, err = u.client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket:          &u.Opt.Bucket,
			Key:             &u.Opt.Remote,
			Body:            file,
			ContentType:     &contentType,
			ACL:             types.ObjectCannedACL(acl),
			Metadata:        metadata,
			CacheControl:    &cacheControl,
			ContentEncoding: &contentEncoding,
		})

		return err
	}

	//nolint:gosec
	hash := md5.New()
	_, _ = io.Copy(hash, file)
	sum := fmt.Sprintf("'%x'", hash.Sum(nil))

	if sum == *head.ETag {
		shouldCopy, reason := u.shouldCopyObject(head, contentType, acl, contentEncoding, cacheControl, metadata)
		if !shouldCopy {
			log.Debug().Msgf("skipping '%s' because hashes and metadata match", u.Opt.Local)

			return nil
		}

		log.Debug().Msgf("updating metadata for '%s' %s", u.Opt.Local, reason)

		if u.Opt.DryRun {
			return nil
		}

		_, err = u.client.CopyObject(context.Background(), &s3.CopyObjectInput{
			Bucket:            &u.Opt.Bucket,
			Key:               &u.Opt.Remote,
			CopySource:        aws.String(fmt.Sprintf("%s/%s", u.Opt.Bucket, u.Opt.Remote)),
			ACL:               types.ObjectCannedACL(acl),
			ContentType:       &contentType,
			Metadata:          metadata,
			MetadataDirective: types.MetadataDirectiveReplace,
			CacheControl:      &cacheControl,
			ContentEncoding:   &contentEncoding,
		})

		return err
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		return err
	}

	log.Debug().Msgf("uploading '%s' with content-type '%s' and permissions '%s'", u.Opt.Local, contentType, acl)

	if u.Opt.DryRun {
		return nil
	}

	_, err = u.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:          &u.Opt.Bucket,
		Key:             &u.Opt.Remote,
		Body:            file,
		ContentType:     &contentType,
		ACL:             types.ObjectCannedACL(acl),
		Metadata:        metadata,
		CacheControl:    &cacheControl,
		ContentEncoding: &contentEncoding,
	})

	return err
}

// shouldCopyObject determines whether an S3 object should be copied based on changes in content type,
// content encoding, cache control, and metadata. It compares the existing object's metadata with the
// provided metadata and returns a boolean indicating whether the object should be copied,
// along with a string describing the reason for the copy if applicable.
//
//nolint:gocognit
func (u *S3Uploader) shouldCopyObject(
	head *s3.HeadObjectOutput, contentType, acl, contentEncoding, cacheControl string, metadata map[string]string,
) (bool, string) {
	var reason string

	if head.ContentType == nil && contentType != "" {
		reason = fmt.Sprintf("content-type has changed from unset to %s", contentType)

		return true, reason
	}

	if head.ContentType != nil && contentType != *head.ContentType {
		reason = fmt.Sprintf("content-type has changed from %s to %s", *head.ContentType, contentType)

		return true, reason
	}

	if head.ContentEncoding == nil && contentEncoding != "" {
		reason = fmt.Sprintf("Content-Encoding has changed from unset to %s", contentEncoding)

		return true, reason
	}

	if head.ContentEncoding != nil && contentEncoding != *head.ContentEncoding {
		reason = fmt.Sprintf("Content-Encoding has changed from %s to %s", *head.ContentEncoding, contentEncoding)

		return true, reason
	}

	if head.CacheControl == nil && cacheControl != "" {
		reason = fmt.Sprintf("cache-control has changed from unset to %s", cacheControl)

		return true, reason
	}

	if head.CacheControl != nil && cacheControl != *head.CacheControl {
		reason = fmt.Sprintf("cache-control has changed from %s to %s", *head.CacheControl, cacheControl)

		return true, reason
	}

	if len(head.Metadata) != len(metadata) {
		reason = fmt.Sprintf("count of metadata values has changed for %s", u.Opt.Local)

		return true, reason
	}

	if len(metadata) > 0 {
		for k, v := range metadata {
			if hv, ok := head.Metadata[k]; ok {
				if v != hv {
					reason = fmt.Sprintf("metadata values have changed for %s", u.Opt.Local)

					return true, reason
				}
			}
		}
	}

	grant, err := u.client.GetObjectAcl(context.TODO(), &s3.GetObjectAclInput{
		Bucket: &u.Opt.Bucket,
		Key:    &u.Opt.Remote,
	})
	if err != nil {
		return false, ""
	}

	previousACL := "private"

	for _, g := range grant.Grants {
		grantee := g.Grantee
		if grantee.URI != nil {
			switch *grantee.URI {
			case "http://acs.amazonaws.com/groups/global/AllUsers":
				if g.Permission == "READ" {
					previousACL = "public-read"
				} else if g.Permission == "WRITE" {
					previousACL = "public-read-write"
				}
			case "http://acs.amazonaws.com/groups/global/AuthenticatedUsers":
				if g.Permission == "READ" {
					previousACL = "authenticated-read"
				}
			}
		}
	}

	if previousACL != acl {
		reason = fmt.Sprintf("permissions for '%s' have changed from '%s' to '%s'", u.Opt.Remote, previousACL, acl)

		return true, reason
	}

	return false, ""
}

// getACL returns the ACL for the given file based on the provided patterns.
func getACL(file string, patterns map[string]string) string {
	for pattern, acl := range patterns {
		if match, _ := filepath.Match(pattern, file); match {
			return acl
		}
	}

	return "private"
}

// getContentType returns the content type for the given file based on the provided patterns.
func getContentType(file string, patterns map[string]string) string {
	ext := filepath.Ext(file)
	if contentType, ok := patterns[ext]; ok {
		return contentType
	}

	return mime.TypeByExtension(ext)
}

// getContentEncoding returns the content encoding for the given file based on the provided patterns.
func getContentEncoding(file string, patterns map[string]string) string {
	ext := filepath.Ext(file)
	if contentEncoding, ok := patterns[ext]; ok {
		return contentEncoding
	}

	return ""
}

// getCacheControl returns the cache control for the given file based on the provided patterns.
func getCacheControl(file string, patterns map[string]string) string {
	for pattern, cacheControl := range patterns {
		if match, _ := filepath.Match(pattern, file); match {
			return cacheControl
		}
	}

	return ""
}

// getMetadata returns the metadata for the given file based on the provided patterns.
func getMetadata(file string, patterns map[string]map[string]string) map[string]string {
	metadata := make(map[string]string)

	for pattern, meta := range patterns {
		if match, _ := filepath.Match(pattern, file); match {
			for k, v := range meta {
				metadata[k] = v
			}

			break
		}
	}

	return metadata
}

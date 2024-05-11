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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cf_types "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3_types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/rs/zerolog/log"
)

type Client struct {
	S3         *S3
	Cloudfront *Cloudfront
}

type Cloudfront struct {
	client       CloudfrontAPIClient
	Distribution string
}

type CloudfrontInvalidateOpt struct {
	Path string
}

type S3 struct {
	client S3APIClient
	Bucket string
	DryRun bool
}

type S3UploadOptions struct {
	LocalFilePath   string
	RemoteObjectKey string
	ACL             map[string]string
	ContentType     map[string]string
	ContentEncoding map[string]string
	CacheControl    map[string]string
	Metadata        map[string]map[string]string
}

type S3RedirectOptions struct {
	Path     string
	Location string
}

type S3DeleteOptions struct {
	RemoteObjectKey string
}

type S3ListOptions struct {
	Path string
}

// NewClient creates a new S3 client with the provided configuration.
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
		S3:         &S3{client: c},
		Cloudfront: &Cloudfront{client: cf},
	}, nil
}

// Upload uploads a file to an S3 bucket. It first checks if the file already exists in the bucket
// and compares the local file's content and metadata with the remote file. If the file has changed,
// it updates the remote file's metadata. If the file does not exist or has changed,
// it uploads the local file to the remote bucket.
func (u *S3) Upload(ctx context.Context, opt S3UploadOptions) error {
	if opt.LocalFilePath == "" {
		return nil
	}

	file, err := os.Open(opt.LocalFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	acl := getACL(opt.LocalFilePath, opt.ACL)
	contentType := getContentType(opt.LocalFilePath, opt.ContentType)
	contentEncoding := getContentEncoding(opt.LocalFilePath, opt.ContentEncoding)
	cacheControl := getCacheControl(opt.LocalFilePath, opt.CacheControl)
	metadata := getMetadata(opt.LocalFilePath, opt.Metadata)

	head, err := u.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &u.Bucket,
		Key:    &opt.RemoteObjectKey,
	})
	if err != nil {
		var noSuchKeyError *s3_types.NoSuchKey
		if !errors.As(err, &noSuchKeyError) {
			return err
		}

		log.Debug().Msgf(
			"'%s' not found in bucket, uploading with content-type '%s' and permissions '%s'",
			opt.LocalFilePath,
			contentType,
			acl,
		)

		if u.DryRun {
			return nil
		}

		_, err = u.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:          &u.Bucket,
			Key:             &opt.RemoteObjectKey,
			Body:            file,
			ContentType:     &contentType,
			ACL:             s3_types.ObjectCannedACL(acl),
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
		shouldCopy, reason := u.shouldCopyObject(
			ctx, head, opt.LocalFilePath, opt.RemoteObjectKey, contentType, acl, contentEncoding, cacheControl, metadata,
		)
		if !shouldCopy {
			log.Debug().Msgf("skipping '%s' because hashes and metadata match", opt.LocalFilePath)

			return nil
		}

		log.Debug().Msgf("updating metadata for '%s' %s", opt.LocalFilePath, reason)

		if u.DryRun {
			return nil
		}

		_, err = u.client.CopyObject(ctx, &s3.CopyObjectInput{
			Bucket:            &u.Bucket,
			Key:               &opt.RemoteObjectKey,
			CopySource:        aws.String(fmt.Sprintf("%s/%s", u.Bucket, opt.RemoteObjectKey)),
			ACL:               s3_types.ObjectCannedACL(acl),
			ContentType:       &contentType,
			Metadata:          metadata,
			MetadataDirective: s3_types.MetadataDirectiveReplace,
			CacheControl:      &cacheControl,
			ContentEncoding:   &contentEncoding,
		})

		return err
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		return err
	}

	log.Debug().Msgf("uploading '%s' with content-type '%s' and permissions '%s'", opt.LocalFilePath, contentType, acl)

	if u.DryRun {
		return nil
	}

	_, err = u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:          &u.Bucket,
		Key:             &opt.RemoteObjectKey,
		Body:            file,
		ContentType:     &contentType,
		ACL:             s3_types.ObjectCannedACL(acl),
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
func (u *S3) shouldCopyObject(
	ctx context.Context, head *s3.HeadObjectOutput,
	local, remote, contentType, acl, contentEncoding, cacheControl string,
	metadata map[string]string,
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
		reason = fmt.Sprintf("count of metadata values has changed for %s", local)

		return true, reason
	}

	if len(metadata) > 0 {
		for k, v := range metadata {
			if hv, ok := head.Metadata[k]; ok {
				if v != hv {
					reason = fmt.Sprintf("metadata values have changed for %s", remote)

					return true, reason
				}
			}
		}
	}

	grant, err := u.client.GetObjectAcl(ctx, &s3.GetObjectAclInput{
		Bucket: &u.Bucket,
		Key:    &remote,
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
		reason = fmt.Sprintf("permissions for '%s' have changed from '%s' to '%s'", remote, previousACL, acl)

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

func (u *S3) Redirect(ctx context.Context, opt S3RedirectOptions) error {
	log.Debug().Msgf("adding redirect from '%s' to '%s'", opt.Path, opt.Location)

	if u.DryRun {
		return nil
	}

	_, err := u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:                  aws.String(u.Bucket),
		Key:                     aws.String(opt.Path),
		ACL:                     s3_types.ObjectCannedACLPublicRead,
		WebsiteRedirectLocation: aws.String(opt.Location),
	})

	return err
}

func (u *S3) Delete(ctx context.Context, opt S3DeleteOptions) error {
	log.Debug().Msgf("removing remote file '%s'", opt.RemoteObjectKey)

	if u.DryRun {
		return nil
	}

	_, err := u.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(u.Bucket),
		Key:    aws.String(opt.RemoteObjectKey),
	})

	return err
}

func (u *S3) List(ctx context.Context, opt S3ListOptions) ([]string, error) {
	remote := make([]string, 0)

	resp, err := u.client.ListObjects(ctx, &s3.ListObjectsInput{
		Bucket: aws.String(u.Bucket),
		Prefix: aws.String(opt.Path),
	})
	if err != nil {
		return remote, err
	}

	for _, item := range resp.Contents {
		remote = append(remote, *item.Key)
	}

	for *resp.IsTruncated {
		resp, err = u.client.ListObjects(ctx, &s3.ListObjectsInput{
			Bucket: aws.String(u.Bucket),
			Prefix: aws.String(opt.Path),
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

func (c *Cloudfront) Invalidate(ctx context.Context, opt CloudfrontInvalidateOpt) error {
	log.Debug().Msgf("invalidating '%s'", opt.Path)

	_, err := c.client.CreateInvalidation(ctx, &cloudfront.CreateInvalidationInput{
		DistributionId: aws.String(c.Distribution),
		InvalidationBatch: &cf_types.InvalidationBatch{
			CallerReference: aws.String(time.Now().Format(time.RFC3339Nano)),
			Paths: &cf_types.Paths{
				Quantity: aws.Int32(1),
				Items: []string{
					opt.Path,
				},
			},
		},
	})

	return err
}

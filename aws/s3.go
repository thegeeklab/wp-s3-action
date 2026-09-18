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
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/rs/zerolog/log"
)

type S3 struct {
	client     S3APIClient
	Bucket     string
	DryRun     bool
	createFile func(string) (io.WriteCloser, error)
}

var (
	ErrEmptyLocalFilePath   = errors.New("local file path is empty")
	ErrLocalPathOutsideRoot = errors.New("local path resolves outside configured root")
	ErrListPagination       = errors.New("invalid s3 list pagination response")
	ErrPartialDelete        = errors.New("s3 delete objects reported per-key failure")
)

// MaxDeleteBatch is the maximum number of keys accepted by a single
// S3 DeleteObjects request.
const MaxDeleteBatch = 1000

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
	RemoteObjectKeys []string
}

type S3DownloadOptions struct {
	LocalRoot       string
	LocalFilePath   string
	RemoteObjectKey string
}

type S3ListOptions struct {
	Path string
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
		var notFoundErr *types.NotFound
		if !errors.As(err, &notFoundErr) {
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
	sum := fmt.Sprintf("%x", hash.Sum(nil))

	// The ETag in HeadObject comes back in different forms depending on
	// the server: simple PUTs return a quoted hex MD5, multipart objects
	// append "-<part-count>" and may or may not be quoted. Comparing the
	// raw string is unreliable; normalize and split multipart suffixes.
	remoteHash, isMultipart := parseETag(aws.ToString(head.ETag))
	hashesMatch := !isMultipart && remoteHash == sum

	if hashesMatch {
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
			ACL:               types.ObjectCannedACL(acl),
			ContentType:       &contentType,
			Metadata:          metadata,
			MetadataDirective: types.MetadataDirectiveReplace,
			CacheControl:      &cacheControl,
			ContentEncoding:   &contentEncoding,
		})

		return err
	}

	// hashes differ OR remote is multipart OR remote ETag is missing.
	// Multipart hashes are not the body MD5 so we cannot verify equality;
	// fall back to a full PutObject.
	if isMultipart {
		log.Debug().Msgf(
			"remote '%s' was uploaded via multipart (etag=%q); cannot verify content match, re-uploading",
			opt.RemoteObjectKey,
			aws.ToString(head.ETag),
		)
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
				//nolint:staticcheck
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

// parseETag normalizes an S3 ETag for content-hash comparison. It
// strips surrounding double or single quotes and splits multipart
// suffixes of the form "-<part-count>" so the returned hash can be
// compared against a body MD5. isMultipart is true when the input had
// such a suffix and the body's MD5 cannot be assumed to match the
// remote's hashing scheme (multipart ETags are derived from MD5s of
// parts, not the body itself).
func parseETag(etag string) (string, bool) {
	etag = strings.TrimFunc(etag, func(r rune) bool {
		return r == '"' || r == '\''
	})

	if i := strings.Index(etag, "-"); i >= 0 {
		return etag[:i], true
	}

	return etag, false
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

// Redirect adds a redirect from the specified path to the specified location in the S3 bucket.
func (u *S3) Redirect(ctx context.Context, opt S3RedirectOptions) error {
	log.Debug().Msgf("adding redirect from '%s' to '%s'", opt.Path, opt.Location)

	if u.DryRun {
		return nil
	}

	_, err := u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:                  aws.String(u.Bucket),
		Key:                     aws.String(opt.Path),
		ACL:                     types.ObjectCannedACLPublicRead,
		WebsiteRedirectLocation: aws.String(opt.Location),
	})

	return err
}

// Delete removes up to 1000 objects from the S3 bucket in a single API call.
// Passing a single key in the slice still issues one HTTP request.
func (u *S3) Delete(ctx context.Context, opt S3DeleteOptions) error {
	if len(opt.RemoteObjectKeys) == 0 {
		return nil
	}

	if u.DryRun {
		for _, key := range opt.RemoteObjectKeys {
			log.Debug().Msgf("removing remote file '%s'", key)
		}

		return nil
	}

	objects := make([]types.ObjectIdentifier, 0, len(opt.RemoteObjectKeys))
	for _, key := range opt.RemoteObjectKeys {
		objects = append(objects, types.ObjectIdentifier{Key: aws.String(key)})
	}

	resp, err := u.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(u.Bucket),
		Delete: &types.Delete{
			Objects: objects,
			Quiet:   aws.Bool(true),
		},
	})
	if err != nil {
		return err
	}

	// Quiet=true elides per-object successes but still returns per-object
	// failures via resp.Errors. Surface them so a partial batch failure
	// is not silently swallowed.
	if len(resp.Errors) > 0 {
		first := resp.Errors[0]

		return fmt.Errorf("%w: %s (%s)", ErrPartialDelete, aws.ToString(first.Key), aws.ToString(first.Message))
	}

	return nil
}

// Download retrieves an object from the S3 bucket and saves it to the local file system.
func (u *S3) Download(ctx context.Context, opt S3DownloadOptions) error {
	if opt.RemoteObjectKey == "" || strings.HasSuffix(opt.RemoteObjectKey, "/") {
		return nil
	}

	if opt.LocalFilePath == "" {
		return ErrEmptyLocalFilePath
	}

	log.Debug().Msgf("downloading remote file '%s' to '%s'", opt.RemoteObjectKey, opt.LocalFilePath)

	if u.DryRun {
		return nil
	}

	resp, err := u.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(u.Bucket),
		Key:    aws.String(opt.RemoteObjectKey),
	})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}

	defer func() {
		// drain and close so the underlying TCP connection returns to the
		// keep-alive pool even on partial reads or errors
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	dir := filepath.Dir(opt.LocalFilePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// re-validate containment after directory creation. Catches symlinks
	// planted in any intermediate component between job build (where the
	// path traversal check ran) and worker execution.
	if err := u.assertPathWithinRoot(opt.LocalRoot, dir); err != nil {
		return err
	}

	createFile := u.createFile
	if createFile == nil {
		createFile = func(name string) (io.WriteCloser, error) {
			return os.Create(name)
		}
	}

	tmpPath := opt.LocalFilePath + ".partial"

	file, err := createFile(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	defer func() {
		// no-op on success because Rename moved tmpPath to LocalFilePath
		_ = os.Remove(tmpPath)
	}()

	_, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()

	if copyErr != nil {
		return fmt.Errorf("write to temp file: %w", copyErr)
	}

	if closeErr != nil {
		return fmt.Errorf("close temp file: %w", closeErr)
	}

	if err := u.assertPathWithinRoot(opt.LocalRoot, tmpPath); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, opt.LocalFilePath); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// assertPathWithinRoot reports an error if path resolves to a location
// outside root. Both arguments must already exist on disk.
func (u *S3) assertPathWithinRoot(root, path string) error {
	if root == "" {
		return nil
	}

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	if resolved != resolvedRoot && !strings.HasPrefix(resolved, resolvedRoot+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s", ErrLocalPathOutsideRoot, resolved)
	}

	return nil
}

// List invokes fn for every object key in the S3 bucket under the
// specified path prefix. The full listing is never materialized, which
// keeps memory bounded for buckets with millions of objects under the
// prefix. fn may return an error to abort iteration.
func (u *S3) List(ctx context.Context, opt S3ListOptions, fn func(string) error) error {
	input := &s3.ListObjectsInput{
		Bucket: aws.String(u.Bucket),
		Prefix: aws.String(opt.Path),
	}

	for {
		resp, err := u.client.ListObjects(ctx, input)
		if err != nil {
			return err
		}

		for _, item := range resp.Contents {
			if err := fn(*item.Key); err != nil {
				return err
			}
		}

		truncated := resp.IsTruncated != nil && *resp.IsTruncated
		if !truncated {
			return nil
		}

		if len(resp.Contents) == 0 {
			return fmt.Errorf("%w: server reported truncated response with empty page", ErrListPagination)
		}

		input.Marker = aws.String(*resp.Contents[len(resp.Contents)-1].Key)
	}
}

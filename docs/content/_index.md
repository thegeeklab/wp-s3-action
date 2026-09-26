---
title: wp-s3-action
---

[![Build Status](https://ci.thegeeklab.de/api/badges/thegeeklab/wp-s3-action/status.svg)](https://ci.thegeeklab.de/repos/thegeeklab/wp-s3-action)
[![Docker Hub](https://img.shields.io/badge/dockerhub-latest-blue.svg?logo=docker&logoColor=white)](https://hub.docker.com/r/thegeeklab/wp-s3-action)
[![Quay.io](https://img.shields.io/badge/quay-latest-blue.svg?logo=docker&logoColor=white)](https://quay.io/repository/thegeeklab/wp-s3-action)
[![GitHub contributors](https://img.shields.io/github/contributors/thegeeklab/wp-s3-action)](https://github.com/thegeeklab/wp-s3-action/graphs/contributors)
[![Source: GitHub](https://img.shields.io/badge/source-github-blue.svg?logo=github&logoColor=white)](https://github.com/thegeeklab/wp-s3-action)
[![License: MIT](https://img.shields.io/github/license/thegeeklab/wp-s3-action)](https://github.com/thegeeklab/wp-s3-action/blob/main/LICENSE)

Woodpecker CI plugin to perform S3 actions.

<!-- prettier-ignore-start -->
<!-- spellchecker-disable -->
{{< toc >}}
<!-- spellchecker-enable -->
<!-- prettier-ignore-end -->

## Usage

```YAML
steps:
  - name: sync
    image: quay.io/thegeeklab/wp-s3-action
    settings:
      action:
        - upload
      access_key: randomstring
      secret_key: random-secret
      region: us-east-1
      bucket: my-bucket.s3-website-us-east-1.amazonaws.com
      source: folder/to/archive
      target: /target/location
```

### Parameters

<!-- prettier-ignore-start -->
<!-- spellchecker-disable -->
{{< propertylist name=wp-s3-action.data sort=name >}}
<!-- spellchecker-enable -->
<!-- prettier-ignore-end -->

### Examples

#### Sync a directory

Actions are executed in the order they are listed. To mirror the local `source` directory with the S3 bucket `target` path, combine `upload` with `upload_delete` to remove remote files that no longer exist locally. Add `redirect` to create redirect objects and `invalidate-cloudfront` to invalidate a CloudFront distribution after the upload. `redirect` requires the server to support website redirects, and `invalidate-cloudfront` only works with AWS CloudFront. Check your provider's documentation before using either action.

```YAML
steps:
  - name: sync
    image: quay.io/thegeeklab/wp-s3-action
    settings:
      action:
        - upload
        - redirect
        - invalidate-cloudfront
      upload_delete: true
      access_key: randomstring
      secret_key: random-secret
      region: us-east-1
      bucket: my-bucket.s3-website-us-east-1.amazonaws.com
      source: folder/to/archive
      target: /target/location
      redirects:
        old/path: https://example.com/new/path
      cloudfront_distribution: E1ABCDEFGHIJKL
```

#### Full cache configuration

Customize `upload_acl`, `upload_content_type`, `upload_content_encoding`, `upload_cache_control` and `upload_metadata`:

```YAML
steps:
  - name: sync
    image: quay.io/thegeeklab/wp-s3-action
    settings:
      action:
        - upload
      access_key: randomstring
      secret_key: random-secret
      region: us-east-1
      bucket: my-bucket.s3-website-us-east-1.amazonaws.com
      source: folder/to/archive
      target: /target/location
      upload_acl:
        "public/*": public-read
        "private/*": private
      upload_content_type:
        ".svg": image/svg+xml
      upload_content_encoding:
        ".js": gzip
        ".css": gzip
      upload_cache_control: "public, max-age: 31536000"
      upload_metadata:
        "*.html":
          Cache-Control: "max-age=3600"
```

All `map` parameters can be specified as `map` for a subset of files or as `string` for all files.

- For the `upload_acl` parameter the key must be a glob. Files without a matching rule will default to `private`.
- For the `upload_content_type` parameter, the key must be a file extension (including the leading dot). To apply a configuration to files without extension, the key can be set to an empty string `""`. For files without a matching rule, the content type is determined automatically.
- For the `upload_content_encoding` parameter, the key must be a file extension (including the leading dot). To apply a configuration to files without extension, the key can be set to an empty string `""`. For files without a matching rule, no Content Encoding header is set.
- For the `upload_cache_control` parameter, the key must be a glob. For files without a matching rule, no Cache Control header is set.

#### Cache a Go build

The Go build cache (`GOCACHE`) consists of thousands of small files. Transferring it object-by-object is slow and generates many S3 requests, so use archive mode to store the whole directory as a single compressed object. Archive mode also preserves symlinks and hard links, which the per-file `download` action does not, so it is safe for the module cache (`GOMODCACHE`) as well.

```YAML
steps:
  - name: restore-cache
    image: quay.io/thegeeklab/wp-s3-action
    settings:
      action:
        - download
      archive: true
      access_key: randomstring
      secret_key: random-secret
      region: us-east-1
      bucket: my-cache-bucket
      source: .cache/go-build
      target: cache/go-build.tar.gz

  - name: build
    image: docker.io/library/golang:1.27
    commands:
      - export GOCACHE="$CI_WORKSPACE/.cache/go-build"
      - go build ./...

  - name: save-cache
    image: quay.io/thegeeklab/wp-s3-action
    settings:
      action:
        - upload
      archive: true
      access_key: randomstring
      secret_key: random-secret
      region: us-east-1
      bucket: my-cache-bucket
      source: .cache/go-build
      target: cache/go-build.tar.gz
```

In archive mode `target` is the full object key rather than a key prefix. Restore and save are each a single GET/PUT of the compressed archive instead of one request per file. On the first run the object does not exist, so the `download` step is a no-op. Use distinct `target` keys to keep multiple caches apart, for example one per Go version or branch. The default compression is `gzip`; set `archive_compression: none` to store an uncompressed tar instead.

#### Sync to a self-hosted S3 server

To sync to a self-hosted S3-compatible server, point `endpoint` at the server and enable `path_style`. Path-style addressing works out of the box, while virtual-hosted-style addressing requires additional DNS configuration.

```YAML
steps:
  - name: sync
    image: quay.io/thegeeklab/wp-s3-action
    settings:
      action:
        - upload
      endpoint: https://s3.example.com
      access_key: randomstring
      secret_key: random-secret
      bucket: my-bucket
      source: folder/to/archive
      target: /target/location
      path_style: true
```

## Build

Build the binary with the following command:

```Shell
make build
```

Build the container image with the following command:

```Shell
docker build --file Containerfile.multiarch --tag thegeeklab/wp-s3-action .
```

## Test

```Shell
docker run --rm \
  -e PLUGIN_ACTION=upload \
  -e PLUGIN_BUCKET=my_bucket \
  -e AWS_ACCESS_KEY_ID=randomstring \
  -e AWS_SECRET_ACCESS_KEY=random-secret \
  -v $(pwd):/build:z \
  -w /build \
  thegeeklab/wp-s3-action
```

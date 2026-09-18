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
      cloudfront_distribution: E2QWRUHAPOMQZL
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
- For the `upload_cache_control` parameter, the key must be a file extension (including the leading dot). If you want to set cache control for files without an extension, set the key to the empty string `""`. For files without a matching rule, no Cache Control header is set.

#### Sync to Minio S3

To use [Minio S3](https://github.com/minio/minio) its required to set `path_style: true`.

```YAML
steps:
  - name: sync
    image: quay.io/thegeeklab/wp-s3-action
    settings:
      action:
        - upload
      endpoint: https://minio.example.com
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

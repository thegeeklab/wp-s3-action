---
title: wp-s3-action
---

[![Build Status](https://ci.thegeeklab.de/api/badges/thegeeklab/wp-s3-action/status.svg)](https://ci.thegeeklab.de/repos/thegeeklab/wp-s3-action)
[![Docker Hub](https://img.shields.io/badge/dockerhub-latest-blue.svg?logo=docker&logoColor=white)](https://hub.docker.com/r/thegeeklab/wp-s3-action)
[![Quay.io](https://img.shields.io/badge/quay-latest-blue.svg?logo=docker&logoColor=white)](https://quay.io/repository/thegeeklab/wp-s3-action)
[![Go Report Card](https://goreportcard.com/badge/github.com/thegeeklab/wp-s3-action)](https://goreportcard.com/report/github.com/thegeeklab/wp-s3-action)
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

**Customize `acl`, `content_type`, `content_encoding` or `cache_control`:**

```YAML
steps:
  - name: sync
    image: quay.io/thegeeklab/wp-s3-action
    settings:
      access_key: randomstring
      secret_key: random-secret
      region: us-east-1
      bucket: my-bucket.s3-website-us-east-1.amazonaws.com
      source: folder/to/archive
      target: /target/location
      acl:
        "public/*": public-read
        "private/*": private
      content_type:
        ".svg": image/svg+xml
      content_encoding:
        ".js": gzip
        ".css": gzip
      cache_control: "public, max-age: 31536000"
```

All `map` parameters can be specified as `map` for a subset of files or as `string` for all files.

- For the `acl` parameter the key must be a glob. Files without a matching rule will default to `private`.
- For the `content_type` parameter, the key must be a file extension (including the leading dot). To apply a configuration to files without extension, the key can be set to an empty string `""`. For files without a matching rule, the content type is determined automatically.
- For the `content_encoding` parameter, the key must be a file extension (including the leading dot). To apply a configuration to files without extension, the key can be set to an empty string `""`. For files without a matching rule, no Content Encoding header is set.
- For the `cache_control` parameter, the key must be a file extension (including the leading dot). If you want to set cache control for files without an extension, set the key to the empty string `""`. For files without a matching rule, no Cache Control header is set.

**Sync to Minio S3:**

To use [Minio S3](https://docs.min.io/) its required to set `path_style: true`.

```YAML
steps:
  - name: sync
    image: quay.io/thegeeklab/wp-s3-action
    settings:
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
  -e PLUGIN_BUCKET=my_bucket \
  -e AWS_ACCESS_KEY_ID=randomstring \
  -e AWS_SECRET_ACCESS_KEY=random-secret \
  -v $(pwd):/build:z \
  -w /build \
  thegeeklab/wp-s3-action
```

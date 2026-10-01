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

#### Customize upload headers and metadata

Customize the headers and user metadata attached to uploaded objects with `upload_acl`, `upload_content_type`, `upload_content_encoding`, `upload_cache_control`, and `upload_metadata`:

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
          Author: "release-team"
```

Each `map` parameter accepts either a map keyed by a pattern that selects a subset of files, or a plain string that applies the same value to every uploaded file:

```YAML
upload_content_encoding: gzip
```

The `map` and `string` forms are mutually exclusive within a single setting.

- For the `upload_acl` parameter the key must be a glob. Files without a matching rule default to `private`.
- For the `upload_content_type` parameter, the key must be a file extension (including the leading dot). To apply a configuration to files without an extension, set the key to an empty string `""`. Files without a matching rule get their content type detected automatically.
- For the `upload_content_encoding` parameter, the key must be a file extension (including the leading dot). To apply a configuration to files without an extension, set the key to an empty string `""`. Files without a matching rule are uploaded without a `Content-Encoding` header.
- For the `upload_cache_control` parameter, the key must be a glob. Files without a matching rule are uploaded without a `Cache-Control` header.
- For the `upload_metadata` parameter, the key must be a glob and the value is a map of user metadata header names to values. S3 stores user metadata under the `x-amz-meta-` prefix, so it is not interchangeable with the standard `Cache-Control`, `Content-Type`, or `Content-Encoding` headers — use `upload_cache_control`, `upload_content_type`, or `upload_content_encoding` for those instead. Only the first matching glob is applied per file.

#### Cache a directory with archive mode

Archive mode packs the `source` directory into a single compressed tar object on S3 instead of transferring each file separately. It is the recommended approach for any directory that contains many small files, e.g. Go build cache (`GOCACHE`), Go module cache (`GOMODCACHE`), `node_modules`, or a Python virtual environment. Archive mode only applies to the `upload` and `download` actions.

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

To cache `GOMODCACHE` alongside `GOCACHE`, run a second restore/save pair that points `source` at the module cache directory and uses a different `target` key (for example `cache/gomod.tar.gz`) so the two archives do not overwrite each other.

In archive mode `target` is the full object key rather than a key prefix, and it must be set for both `upload` and `download`. Restore and save are each a single GET or PUT of the compressed archive. If the object does not yet exist, the `download` step logs the miss and exits successfully, so the first build against a fresh cache key simply runs with a cold cache. Restored files keep their original permissions and modification times, which matters for caches such as `GOCACHE` that rely on them to detect reuse.

Keep caches for different inputs apart with distinct `target` keys, for example one per Go version, branch, or pipeline. The key is rendered as a Go template against the [wp-plugin-go Metadata](https://pkg.go.dev/github.com/thegeeklab/wp-plugin-go@main/plugin#Metadata), so you can derive it automatically:

```YAML
target: cache/{{ .Repository.Branch }}-{{ .Pipeline.Number }}.tar.gz
```

Common fields include `.Repository.Name`, `.Repository.Owner`, `.Repository.Branch`, `.Pipeline.Number`, `.Pipeline.Event`, and `.Curr.Sha`. The template also exposes the [sprig](https://masterminds.github.io/sprig/) function map plus `sha256file`, which returns the SHA-256 digest of one or more files. Key the cache on a lockfile instead of a pipeline identifier to invalidate it automatically when dependencies change:

```YAML
target: cache/go-build-{{ sha256file "go.sum" }}.tar.gz
```

Pass several paths to hash them in order, for example `sha256file "go.mod" "go.sum"`. `sha256file` reads each path relative to the workspace and fails the step if a path does not exist, so only reference files that are checked into the repository or generated by an earlier step. Templating applies only in archive mode.

The default compression is `gzip`. Set `archive_compression: none` to store an uncompressed tar instead. The archive is streamed directly between the local filesystem and S3.

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

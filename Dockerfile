# ---- Stage 0 ----
# Builds media repo binaries
FROM golang:1.26-alpine3.23 AS builder

# Install build dependencies
RUN apk add --no-cache git build-base pkgconf libheif-dev

WORKDIR /opt
COPY . /opt

# Run remaining build steps
RUN ./build.sh

# ---- Stage 1 ----
# Shared runtime base.
FROM alpine:3.23 AS runtime-base

RUN mkdir /plugins
RUN apk add --no-cache \
        ca-certificates \
        libheif

COPY --from=builder /opt/bin/plugin_antispam_ocr /plugins/
COPY --from=builder \
 /opt/bin/media_repo \
 /opt/bin/import_synapse \
 /opt/bin/import_dendrite \
 /opt/bin/export_synapse_for_import \
 /opt/bin/export_dendrite_for_import \
 /opt/bin/import_to_synapse \
 /opt/bin/gdpr_export \
 /opt/bin/gdpr_import \
 /opt/bin/s3_consistency_check \
 /opt/bin/combine_signing_keys \
 /opt/bin/generate_signing_key \
 /opt/bin/thumbnailer \
 /usr/local/bin/

COPY ./config.sample.yaml /etc/media-repo.yaml.sample

WORKDIR /data

ENV REPO_CONFIG=/data/media-repo.yaml

VOLUME ["/data", "/media"]
EXPOSE 8000
CMD ["media_repo"]

# ---- Stage 2 ----
# Default slim runtime with SVG/JXL support, but without ffmpeg.
FROM runtime-base AS runtime-slim

RUN apk add --no-cache \
		rsvg-convert \
		libjxl-tools

# ---- Stage 3 ----
# Full runtime adds MP4 thumbnailing support.
FROM runtime-slim AS runtime-full

RUN apk add --no-cache \
		ffmpeg

# ---- Stage 4 ----
# Default final image target.
FROM runtime-slim AS runtime

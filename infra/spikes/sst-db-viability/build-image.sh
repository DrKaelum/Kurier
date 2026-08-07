#!/usr/bin/env sh
set -eu

service_dir=$(CDPATH= cd -- "$(dirname -- "$0")/service" && pwd)
artifact_dir="$service_dir/.build"

mkdir -p "$artifact_dir"
curl \
  --fail \
  --location \
  --retry 3 \
  --show-error \
  --silent \
  --output "$artifact_dir/ca-certificates.crt" \
  https://curl.se/ca/cacert.pem
curl \
  --fail \
  --location \
  --retry 3 \
  --show-error \
  --silent \
  --output "$artifact_dir/rds-global-bundle.pem" \
  https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem

(
  cd "$service_dir"
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
    go build -trimpath -ldflags="-s -w" \
    -o "$artifact_dir/db-viability-service" .
)

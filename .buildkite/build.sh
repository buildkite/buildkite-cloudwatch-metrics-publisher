#!/bin/bash
set -eu

echo "--- Building binary"
docker run --rm \
  --volume "$PWD":/go/src/github.com/buildkite/buildkite-cloudwatch-metrics-publisher \
  --workdir /go/src/github.com/buildkite/buildkite-cloudwatch-metrics-publisher \
  golang:1.6.0 make build/buildkite-cloudwatch-metrics


echo "--- Uploading artifacts to :buildkite:"
buildkite-agent artifact upload build/buildkite-cloudwatch-metrics
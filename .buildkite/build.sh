#!/bin/bash
set -eux
docker run --rm \
  --volume "$PWD":/go/src/github.com/buildkite/buildkite-cloudwatch-metrics-publisher \
  --workdir /go/src/github.com/buildkite/buildkite-cloudwatch-metrics-publisher \
  golang:1.6.0 make build/buildkite-cloudwatch-metrics

buildkite-agent artifact upload build/buildkite-cloudwatch-metrics
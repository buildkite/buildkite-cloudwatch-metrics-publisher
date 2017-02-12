#!/bin/bash

set -eu

buildkite-agent artifact download "build/*" build/

make upload branch="${BUILDKITE_BRANCH}"

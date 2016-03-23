#!/bin/bash
set -eu

buildkite-agent artifact download build/ build/
ls -al build/
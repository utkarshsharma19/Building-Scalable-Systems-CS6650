#!/usr/bin/env bash
# Build the Part III Lambda: a Linux binary named `bootstrap`, zipped.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p build
(cd src && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -ldflags="-s -w" \
  -o ../build/bootstrap ./cmd/lambda)
rm -f build/lambda.zip
(cd build && zip -q -X lambda.zip bootstrap)
ls -lh build/lambda.zip

#!/usr/bin/env bash
# Creates the test files (random bytes, so corruption is detectable) and uploads them to S3.
# usage: scripts/make_files.sh            (needs env.sh sourced and the stack deployed)
set -euo pipefail
source "$(dirname "$0")/env.sh" >/dev/null
DIR=$PART2/testfiles
mkdir -p "$DIR"
# name:bytes. 64-byte chunks, so 1 KiB = 16 chunks ... 1 MiB = 16384 chunks.
for spec in f1k:1024 f16k:16384 f64k:65536 f256k:262144 f1m:1048576; do
	name=${spec%%:*}.bin
	size=${spec#*:}
	[ -f "$DIR/$name" ] || head -c "$size" /dev/urandom >"$DIR/$name"
done
(cd "$DIR" && shasum -a 256 *.bin >SHA256SUMS)
aws s3 cp "$DIR" "s3://$BUCKET/$PREFIX" --recursive --exclude "*" --include "*.bin" --only-show-errors
aws s3 ls "s3://$BUCKET/$PREFIX"

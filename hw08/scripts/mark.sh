#!/usr/bin/env bash
# Append a timestamped marker to results/timeline.csv: scripts/mark.sh <event> <start|end>
cd "$(dirname "$0")/.."
echo "$1,$2,$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> results/timeline.csv

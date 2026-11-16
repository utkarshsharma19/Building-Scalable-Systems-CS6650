#!/usr/bin/env bash
# 100 users for 5 minutes, spawn 10/s, 4 worker processes (same as Eason's STEP II run).
# usage: scripts/locust-run.sh BASE_URL LABEL
set -euo pipefail
cd "$(dirname "$0")/.."
scripts/mark.sh "locust-$2" start
.venv/bin/locust -f locust/locustfile.py --headless -H "$1" -u 100 -r 10 -t 5m --processes 4 \
  --csv "results/locust-$2" --only-summary 2>&1 | tail -25 > "results/locust-$2-summary.txt"
scripts/mark.sh "locust-$2" end
cat "results/locust-$2-summary.txt"

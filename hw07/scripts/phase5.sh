#!/usr/bin/env bash
# Phase 5: for each worker count, redeploy the processor, run the flash sale,
# and sample the queue until it drains.   scripts/phase5.sh 100 20 5
set -euo pipefail
cd "$(dirname "$0")/.."
url=$(terraform -chdir=terraform output -raw queue_url)

for n in "$@"; do
  echo "=== $n workers: deploying"
  terraform -chdir=terraform apply -auto-approve -input=false -no-color -var processor_workers="$n" | grep -E "Apply complete|Error"
  aws ecs wait services-stable --cluster hw7-cluster --services hw7-processor
  out=results/p5-w$n
  mkdir -p "$out"
  scripts/queue-watch.sh "$out/queue.csv" > "$out/queue-watch.log" 2>&1 &
  watcher=$!
  sleep 6
  start=$(date +%s)
  scripts/run-test.sh "p5-w$n" AsyncUser 20 10 60s | grep SUMMARY
  echo "test ended $(date +%T); waiting for the queue to drain"
  wait "$watcher" || true
  tail -1 "$out/queue-watch.log"
  aws logs tail /ecs/hw7-processor --since "$(( $(date +%s) - start + 60 ))s" --format short |
    grep '"stats"' > "$out/processor-stats.log" || true
  echo "$(date -u +%FT%TZ) workers=$n start_epoch=$start" >> "$out/meta.txt"
done

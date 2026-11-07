#!/usr/bin/env bash
# Sample the SQS queue every 5 s into a CSV, until the queue has been empty
# for a minute (or Ctrl-C). Gives exact peak depth and drain time; the
# CloudWatch metric is 1-minute resolution and lags a few minutes.
#
#   scripts/queue-watch.sh results/p5-workers-20/queue.csv
set -euo pipefail
cd "$(dirname "$0")/.."
out=${1:-results/queue.csv}
mkdir -p "$(dirname "$out")"
url=$(terraform -chdir=terraform output -raw queue_url)

echo "time,epoch,visible,in_flight" > "$out"
empty=0 seen=0 peak=0
while true; do
  read -r vis inf < <(aws sqs get-queue-attributes --queue-url "$url" \
    --attribute-names ApproximateNumberOfMessages ApproximateNumberOfMessagesNotVisible \
    --query 'Attributes.[ApproximateNumberOfMessages,ApproximateNumberOfMessagesNotVisible]' --output text)
  now=$(date +%s)
  echo "$(date -u +%T),$now,$vis,$inf" >> "$out"
  (( vis > peak )) && peak=$vis
  (( vis + inf > 0 )) && seen=1
  printf '%s visible=%-6s in_flight=%-4s peak=%s\n' "$(date +%T)" "$vis" "$inf" "$peak"
  if (( seen && vis + inf == 0 )); then
    (( ++empty >= 12 )) && break
  else
    empty=0
  fi
  sleep 5
done
echo "peak visible=$peak; queue empty since $(date -r $((now - 55)) +%T) (CSV: $out)"

#!/usr/bin/env bash
# Part III: send a few orders with curl (NOT Locust), then print the Lambda's
# REPORT lines. Lines with "Init Duration" are cold starts.
#
#   scripts/lambda-orders.sh send 5     # 5 orders, 2 s apart
#   scripts/lambda-orders.sh burst 5    # 5 orders at once (each may get its own cold start)
#   scripts/lambda-orders.sh report     # REPORT lines from the last 60 minutes
set -euo pipefail
cd "$(dirname "$0")/.."
mode=${1:-report} n=${2:-5}
alb=$(terraform -chdir=terraform output -raw alb_url)
group=$(terraform -chdir=terraform output -raw lambda_log_group)

send() {
  curl -s -o /dev/null -w "%{http_code} %{time_total}s\n" -X POST "$alb/orders/async" \
    -H "Content-Type: application/json" \
    -d "{\"customer_id\": $((RANDOM % 1000 + 1)), \"items\": [{\"product_id\": \"p001\", \"quantity\": 1, \"price\": 19.99}]}"
}

case $mode in
  send)  for ((i = 1; i <= n; i++)); do echo -n "$(date +%T) order $i: "; send; sleep 2; done ;;
  burst) for ((i = 1; i <= n; i++)); do send & done; wait ;;
  report)
    aws logs filter-log-events --log-group-name "$group" --filter-pattern '"REPORT RequestId"' \
      --start-time $(( ($(date +%s) - 3600) * 1000 )) --query 'events[].[timestamp,message]' --output text |
      while IFS=$'\t' read -r ts msg; do
        kind=warm; [[ $msg == *"Init Duration"* ]] && kind=COLD
        echo "$(date -r $((ts / 1000)) +%T) $kind $(echo "$msg" | tr '\t' ' ' | sed 's/RequestId: [^ ]* //')"
      done ;;
esac

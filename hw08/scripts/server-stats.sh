#!/usr/bin/env bash
# Server-side (in the service) latency per operation from the request logs, via CloudWatch Logs Insights.
# usage: scripts/server-stats.sh START_ISO END_ISO OUT.json [END_PADDING_S=60]
set -euo pipefail
cd "$(dirname "$0")/.."
LG=$(terraform -chdir=terraform output -raw log_group)
s=$(date -j -u -f %Y-%m-%dT%H:%M:%SZ "$1" +%s); e=$(date -j -u -f %Y-%m-%dT%H:%M:%SZ "$2" +%s)
Q='fields op, total_ms, db_ms | filter msg = "request" and ispresent(op)
| stats count(*) as n, avg(total_ms) as svc_avg, pct(total_ms, 50) as svc_p50, pct(total_ms, 95) as svc_p95, pct(total_ms, 99) as svc_p99,
        avg(db_ms) as db_avg, pct(db_ms, 50) as db_p50, pct(db_ms, 95) as db_p95, pct(db_ms, 99) as db_p99, max(db_ms) as db_max by op'
qid=$(aws logs start-query --profile hw8 --log-group-name "$LG" --start-time "$s" --end-time "$((e+${4:-60}))" --query-string "$Q" --query queryId --output text)
for _ in $(seq 60); do
  out=$(aws logs get-query-results --profile hw8 --query-id "$qid")
  st=$(echo "$out" | python3 -c 'import sys,json;print(json.load(sys.stdin)["status"])')
  [ "$st" = "Complete" ] && break; sleep 2
done
echo "$out" | python3 -c '
import sys,json
d=json.load(sys.stdin)
rows=[{c["field"]:c["value"] for c in r} for r in d["results"]]
json.dump(rows, open(sys.argv[1],"w"), indent=2)
for r in rows: print(r)' "$3"

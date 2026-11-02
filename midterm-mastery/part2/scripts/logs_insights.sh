#!/usr/bin/env bash
# Pulls the experiment log lines I added (FETCH_STATS from the gateway, BACKEND_STATS from the
# backend) for a time window out of CloudWatch Logs Insights and saves them as CSV.
# usage: scripts/logs_insights.sh LABEL START_EPOCH END_EPOCH
set -euo pipefail
source "$(dirname "$0")/env.sh" >/dev/null
LABEL=$1 START=$2 END=$3

run() { # log-group query outfile
	local id status
	id=$(aws logs start-query --log-group-name "$1" --start-time "$START" --end-time "$END" \
		--query-string "$2" --query queryId --output text)
	for _ in $(seq 1 60); do
		status=$(aws logs get-query-results --query-id "$id" --query status --output text)
		[ "$status" = Complete ] && break
		sleep 2
	done
	aws logs get-query-results --query-id "$id" --output json | python3 -c '
import json, sys, csv
rows = json.load(sys.stdin)["results"]
if not rows: sys.exit(0)
keys = [f["field"] for f in rows[0] if f["field"] != "@ptr"]
w = csv.writer(sys.stdout); w.writerow(keys)
for r in rows:
    d = {f["field"]: f["value"] for f in r}
    w.writerow([d.get(k, "") for k in keys])' >"$3"
	echo "wrote $3 ($(($(wc -l <"$3") - 1)) rows)"
}

run /ecs/file-transfer-gateway 'filter @message like /FETCH_STATS/
| parse @message /file=(?<file>\S+) chunks=(?<chunks>\d+) open_attempts=(?<opens>\d+) retry_rounds=(?<rounds>\d+) retys_sent=(?<retys>\d+) missing_final=(?<missing>\d+) dur_ms=(?<dur>\d+) ok=(?<ok>\w+)/
| stats count(*) as fetches, avg(opens) as avg_open_attempts, avg(rounds) as avg_retry_rounds, avg(retys) as avg_retys_sent, max(retys) as max_retys_sent, avg(missing) as avg_missing_final, avg(dur) as avg_ms, max(dur) as max_ms by file, ok
| sort file, ok' "$RESULTS/${LABEL}_gateway_fetch_stats.csv"

run /ecs/file-transfer-backend 'filter @message like /BACKEND_STATS/
| parse @message /opens=(?<o>\d+) retys=(?<r>\d+) source_fetches=(?<f>\d+) avg_fetch_ms=(?<fms>\d+) datagrams_sent=(?<sent>\d+) overload_drops=(?<drops>\d+) queue_high=(?<qh>\d+)/
| stats sum(o) as opens, sum(r) as retys, sum(f) as s3_fetches, avg(fms) as avg_s3_fetch_ms, sum(sent) as datagrams_sent, sum(drops) as overload_drops, max(qh) as max_queue_depth' "$RESULTS/${LABEL}_backend_stats.csv"

#!/usr/bin/env bash
# Experiment 3: what the two auto scalers do under a step load (5 -> 40 -> 1 users, 28 min).
# Default scaling settings: gateway ALBRequestCountPerTarget=100/min, backend CPU=60%, 2..4 tasks.
# usage: scripts/exp3_autoscale.sh
set -euo pipefail
source "$(dirname "$0")/env.sh" >/dev/null
start=$(date +%s)
"$PART2/scripts/scale_watch.sh" "$RESULTS/exp3_scale_timeline.csv" >/dev/null &
WATCH=$!
trap 'kill $WATCH 2>/dev/null' EXIT
SHAPE=step FILE=f64k.bin "$VENV/bin/locust" -f "$PART2/locust/locustfile.py" --headless -H "$GATEWAY" \
	--csv "$RESULTS/exp3" --csv-full-history --html "$RESULTS/exp3.html" --only-summary || true
sleep 60
end=$(date +%s)
echo "exp3,$start,$end" >"$RESULTS/exp3_window.csv"
"$PART2/scripts/cloudwatch_metrics.sh" exp3 "$start" "$end"
"$PART2/scripts/logs_insights.sh" exp3 "$start" "$end"

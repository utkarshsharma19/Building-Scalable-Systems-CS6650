#!/usr/bin/env bash
# Experiment 2: gateway MAX_INFLIGHT vs. offered load (auto scaling pinned at 2+2 tasks).
# Run once per deployed MAX_INFLIGHT value; each call runs Locust for each user count.
# usage: scripts/exp2_inflight.sh INFLIGHT [users...]     e.g. scripts/exp2_inflight.sh 4 10 40
set -euo pipefail
source "$(dirname "$0")/env.sh" >/dev/null
INFLIGHT=$1
shift
USERS=${*:-10 40}
# Locust's CSV writer blocks its event loop on slow disk writes (my Desktop is iCloud-synced,
# which froze all users in the first attempt), so it writes to a local temp folder first.
LOUT=${LOCUST_OUT:-$HOME/.cache/midterm-locust}
mkdir -p "$LOUT"
for u in $USERS; do
	label=exp2_inflight${INFLIGHT}_u${u}
	start=$(date +%s)
	echo "== $label (start $start)"
	FILE=f16k.bin "$VENV/bin/locust" -f "$PART2/locust/locustfile.py" --headless -H "$GATEWAY" \
		-u "$u" -r "$u" -t 180s --csv "$LOUT/$label" --html "$LOUT/$label.html" --only-summary || true
	cp "$LOUT/$label"* "$RESULTS/"
	end=$(date +%s)
	echo "$label,$INFLIGHT,$u,$start,$end" >>"$RESULTS/exp2_runs.csv"
	sleep 70 # let CloudWatch log ingestion catch up and the queues drain before the next level
	"$PART2/scripts/logs_insights.sh" "$label" "$start" "$end"
done

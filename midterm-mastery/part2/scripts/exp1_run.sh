#!/usr/bin/env bash
# Experiment 1, one loss setting: run the sequential sweep, then pull the per-fetch retry stats.
# The backend must already be deployed with the same loss, e.g.
#   (cd $SERVICE_REPO && scripts/deploy.sh --yes --skip-build --var backend_send_loss=5)
# usage: scripts/exp1_run.sh SEND_LOSS RECV_LOSS [reps]
set -euo pipefail
source "$(dirname "$0")/env.sh" >/dev/null
S=$1 R=$2 REPS=${3:-8}
label=exp1_s${S}_r${R}
start=$(date +%s)
"$VENV/bin/python" "$PART2/scripts/exp1_loss_sweep.py" --label "$label" --send-loss "$S" --recv-loss "$R" --reps "$REPS"
end=$(date +%s)
sleep 60
"$PART2/scripts/logs_insights.sh" "$label" "$start" "$end"

#!/usr/bin/env bash
# Runs Experiment 2 and the lossy half of Experiment 1, redeploying between settings.
# Scaling is pinned (gateway 2..2, backend 2..2) so task count is a fixed variable.
set -euo pipefail
source "$(dirname "$0")/env.sh" >/dev/null
deploy() { (cd "$SERVICE_REPO" && scripts/deploy.sh --yes --skip-build --var gateway_max=2 --var backend_max=2 "$@" | tail -8); }

echo "### Exp2: MAX_INFLIGHT=4"
deploy --var max_inflight=4
"$PART2/scripts/exp2_inflight.sh" 4 10 40
echo "### Exp2: MAX_INFLIGHT=16"
deploy --var max_inflight=16
"$PART2/scripts/exp2_inflight.sh" 16 10 40

for cfg in "1 0" "5 0" "0 20"; do
	set -- $cfg
	echo "### Exp1: send_loss=$1 recv_loss=$2"
	deploy --var max_inflight=4 --var backend_send_loss=$1 --var backend_recv_loss=$2
	"$PART2/scripts/exp1_run.sh" "$1" "$2" 8
done
echo PHASES_2_3_DONE

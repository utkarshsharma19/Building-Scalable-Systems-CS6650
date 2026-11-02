#!/usr/bin/env bash
# Polls both ECS services every 15 s and writes a timeline CSV: desired/running task counts and
# healthy targets for the gateway (ALB) and the backend (NLB). Stop with Ctrl-C.
# usage: scripts/scale_watch.sh results/exp3_scale_timeline.csv
set -uo pipefail
source "$(dirname "$0")/env.sh" >/dev/null
OUT=${1:-$RESULTS/scale_timeline-$(date +%Y%m%d-%H%M%S).csv}
GW_TG=$(aws elbv2 describe-target-groups --names file-transfer-gateway --query 'TargetGroups[0].TargetGroupArn' --output text)
BE_TG=$(aws elbv2 describe-target-groups --names file-transfer-backend --query 'TargetGroups[0].TargetGroupArn' --output text)
healthy() { aws elbv2 describe-target-health --target-group-arn "$1" --query 'length(TargetHealthDescriptions[?TargetHealth.State==`healthy`])' --output text; }

echo "epoch,time,gw_desired,gw_running,gw_healthy,be_desired,be_running,be_healthy" | tee "$OUT"
while true; do
	read -r gd gr bd br < <(aws ecs describe-services --cluster "$CLUSTER" --services gateway backend \
		--query 'services[].[desiredCount,runningCount]' --output text | tr '\n' ' ')
	echo "$(date +%s),$(date +%H:%M:%S),$gd,$gr,$(healthy "$GW_TG"),$bd,$br,$(healthy "$BE_TG")" | tee -a "$OUT"
	sleep 15
done

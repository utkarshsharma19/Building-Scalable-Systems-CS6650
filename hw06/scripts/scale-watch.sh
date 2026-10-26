#!/usr/bin/env bash
# Usage: ./scripts/scale-watch.sh [out.csv]
set -uo pipefail
cd "$(dirname "$0")/../terraform"

CLUSTER=$(terraform output -raw ecs_cluster_name)
SERVICE=$(terraform output -raw ecs_service_name)
TG=$(terraform output -raw target_group_arn 2>/dev/null || true)
OUT=${1:-../results/scale-timeline-$(date +%Y%m%d-%H%M%S).csv}
mkdir -p "$(dirname "$OUT")"

echo "time,desired,running,pending,healthy,unhealthy,draining,initial" | tee "$OUT"
while true; do
  read -r desired running pending < <(aws ecs describe-services --cluster "$CLUSTER" --services "$SERVICE" \
    --query 'services[0].[desiredCount,runningCount,pendingCount]' --output text)
  healthy=0 unhealthy=0 draining=0 initial=0
  if [[ -n "$TG" && "$TG" != "null" ]]; then
    states=$(aws elbv2 describe-target-health --target-group-arn "$TG" \
      --query 'TargetHealthDescriptions[].TargetHealth.State' --output text)
    for s in $states; do
      case $s in
        healthy) ((healthy++)) ;; unhealthy) ((unhealthy++)) ;;
        draining) ((draining++)) ;; initial) ((initial++)) ;;
      esac
    done
  fi
  echo "$(date +%H:%M:%S),$desired,$running,$pending,$healthy,$unhealthy,$draining,$initial" | tee -a "$OUT"
  sleep 15
done

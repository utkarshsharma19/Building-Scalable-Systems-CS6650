#!/usr/bin/env bash
# Usage: ./scripts/stop-tasks.sh [N]
set -euo pipefail
cd "$(dirname "$0")/../terraform"
N=${1:-1}

CLUSTER=$(terraform output -raw ecs_cluster_name)
SERVICE=$(terraform output -raw ecs_service_name)

for arn in $(aws ecs list-tasks --cluster "$CLUSTER" --service-name "$SERVICE" \
    --desired-status RUNNING --query 'taskArns' --output text | tr '\t' '\n' | head -n "$N"); do
  echo "$(date +%H:%M:%S) stopping ${arn##*/}"
  aws ecs stop-task --cluster "$CLUSTER" --task "$arn" --reason "HW6 resilience test" \
    --query 'task.lastStatus' --output text
done

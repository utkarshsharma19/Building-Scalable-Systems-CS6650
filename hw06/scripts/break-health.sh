#!/usr/bin/env bash
# Usage: ./scripts/break-health.sh on|off
set -euo pipefail
cd "$(dirname "$0")/../terraform"
TG=$(terraform output -raw target_group_arn)

case ${1:-} in
  on)  path=/health-broken ;;
  off) path=/health ;;
  *)   echo "usage: $0 on|off" >&2; exit 1 ;;
esac
aws elbv2 modify-target-group --target-group-arn "$TG" --health-check-path "$path" \
  --query 'TargetGroups[0].HealthCheckPath' --output text

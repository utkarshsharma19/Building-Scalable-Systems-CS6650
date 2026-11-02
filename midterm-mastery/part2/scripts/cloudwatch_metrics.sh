#!/usr/bin/env bash
# Saves the CloudWatch metrics the two scalers look at (1-minute points) for a time window:
# ECS CPU for both services and the ALB's RequestCountPerTarget / 5xx counts for the gateway.
# usage: scripts/cloudwatch_metrics.sh LABEL START_EPOCH END_EPOCH
set -euo pipefail
source "$(dirname "$0")/env.sh" >/dev/null
LABEL=$1 START=$2 END=$3
TF="terraform -chdir=$SERVICE_REPO/terraform"
ALB=$(aws elbv2 describe-load-balancers --names file-transfer-gateway --query 'LoadBalancers[0].LoadBalancerArn' --output text | sed 's#.*:loadbalancer/##')
TG=$(aws elbv2 describe-target-groups --names file-transfer-gateway --query 'TargetGroups[0].TargetGroupArn' --output text | sed 's#.*:##')
q() { # id namespace metric stat dims...
	local id=$1 ns=$2 m=$3 st=$4
	shift 4
	printf '{"Id":"%s","MetricStat":{"Metric":{"Namespace":"%s","MetricName":"%s","Dimensions":[%s]},"Period":60,"Stat":"%s"}}' \
		"$id" "$ns" "$m" "$*" "$st"
}
d() { printf '{"Name":"%s","Value":"%s"}' "$1" "$2"; }
Q="[$(q gw_cpu AWS/ECS CPUUtilization Average "$(d ClusterName $CLUSTER),$(d ServiceName gateway)"),
$(q be_cpu AWS/ECS CPUUtilization Average "$(d ClusterName $CLUSTER),$(d ServiceName backend)"),
$(q req_per_target AWS/ApplicationELB RequestCountPerTarget Sum "$(d TargetGroup "$TG")"),
$(q requests AWS/ApplicationELB RequestCount Sum "$(d LoadBalancer "$ALB")"),
$(q target_5xx AWS/ApplicationELB HTTPCode_Target_5XX_Count Sum "$(d LoadBalancer "$ALB")"),
$(q target_2xx AWS/ApplicationELB HTTPCode_Target_2XX_Count Sum "$(d LoadBalancer "$ALB")"),
$(q resp_time AWS/ApplicationELB TargetResponseTime Average "$(d LoadBalancer "$ALB")")]"
aws cloudwatch get-metric-data --start-time "$START" --end-time "$END" --metric-data-queries "$Q" \
	--output json >"$RESULTS/${LABEL}_cloudwatch.json"
echo "wrote $RESULTS/${LABEL}_cloudwatch.json"
aws application-autoscaling describe-scaling-activities --service-namespace ecs --output json \
	--query 'ScalingActivities[].{t:StartTime,res:ResourceId,desc:Description,cause:Cause,status:StatusCode}' \
	>"$RESULTS/${LABEL}_scaling_activities.json"
echo "wrote $RESULTS/${LABEL}_scaling_activities.json"

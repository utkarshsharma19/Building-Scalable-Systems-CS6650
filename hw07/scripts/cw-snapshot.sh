#!/usr/bin/env bash
# Render real CloudWatch graphs to PNG (same images the console draws).
#
#   scripts/cw-snapshot.sh <out-prefix> <start ISO | -PT90M> [end ISO]
#   scripts/cw-snapshot.sh screenshots/p5-w20 2026-11-07T21:50:00Z 2026-11-07T22:05:00Z
set -euo pipefail
cd "$(dirname "$0")/.."
prefix=$1 start=$2 end=${3:-}
endjson=""; [[ -n $end ]] && endjson=",\"end\":\"$end\""
mkdir -p "$(dirname "$prefix")"

render() { # name title period metrics-json [yaxis-json]
  local name=$1 title=$2 period=$3 metrics=$4 yaxis=${5:-'{}'}
  local w="{\"title\":\"$title\",\"view\":\"timeSeries\",\"stacked\":false,\"width\":900,\"height\":400,
    \"period\":$period,\"start\":\"$start\"$endjson,\"region\":\"us-west-2\",\"metrics\":$metrics,\"yAxis\":$yaxis}"
  aws cloudwatch get-metric-widget-image --metric-widget "$w" --output-format png \
    --query MetricWidgetImage --output text | base64 --decode > "${prefix}-${name}.png"
  echo "wrote ${prefix}-${name}.png"
}

case ${WHAT:-sqs} in
  sqs)
    render sqs-depth "SQS ApproximateNumberOfMessagesVisible (order-processing-queue)" 60 \
      '[["AWS/SQS","ApproximateNumberOfMessagesVisible","QueueName","order-processing-queue",{"stat":"Maximum","label":"Visible (max/min)"}]]'
    render sqs-flow "SQS messages sent vs deleted per minute" 60 \
      '[["AWS/SQS","NumberOfMessagesSent","QueueName","order-processing-queue",{"stat":"Sum","label":"Sent (orders in)"}],["AWS/SQS","NumberOfMessagesDeleted","QueueName","order-processing-queue",{"stat":"Sum","label":"Deleted (orders processed)"}]]'
    render ecs-processor "Order Processor task CPU and memory (%)" 60 \
      '[["AWS/ECS","CPUUtilization","ClusterName","hw7-cluster","ServiceName","hw7-processor",{"stat":"Maximum","label":"CPU % (max)"}],["AWS/ECS","MemoryUtilization","ClusterName","hw7-cluster","ServiceName","hw7-processor",{"stat":"Maximum","label":"Memory % (max)"}]]' \
      '{"left":{"min":0,"max":100}}'
    ;;
  alb)
    render alb "ALB requests and 5xx per minute" 60 \
      '[["AWS/ApplicationELB","RequestCount","LoadBalancer","'"$ALB_SUFFIX"'",{"stat":"Sum","label":"Requests"}],["AWS/ApplicationELB","HTTPCode_ELB_5XX_Count","LoadBalancer","'"$ALB_SUFFIX"'",{"stat":"Sum","label":"ELB 5xx"}]]'
    render alb-latency "ALB TargetResponseTime (s)" 60 \
      '[["AWS/ApplicationELB","TargetResponseTime","LoadBalancer","'"$ALB_SUFFIX"'",{"stat":"p50","label":"p50"}],["AWS/ApplicationELB","TargetResponseTime","LoadBalancer","'"$ALB_SUFFIX"'",{"stat":"Maximum","label":"max"}]]'
    ;;
  lambda)
    render lambda "Lambda invocations and duration" 60 \
      '[["AWS/Lambda","Invocations","FunctionName","hw7-order-processor",{"stat":"Sum","label":"Invocations"}],["AWS/Lambda","Duration","FunctionName","hw7-order-processor",{"stat":"Maximum","label":"Duration max (ms)","yAxis":"right"}]]'
    ;;
esac

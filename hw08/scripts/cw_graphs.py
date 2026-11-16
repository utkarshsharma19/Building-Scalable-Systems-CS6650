#!/usr/bin/env python3
"""Render CloudWatch metric graphs to PNG with `aws cloudwatch get-metric-widget-image`.

usage: cw_graphs.py SET START_ISO END_ISO PREFIX
  SET = dynamodb | ecs | alb | insights | rds
Writes screenshots/PREFIX-<name>.png
"""
import base64
import json
import subprocess
import sys

PROFILE, REGION = "hw8", "us-west-2"


def tf_out(name):
    return subprocess.check_output(["terraform", "-chdir=terraform", "output", "-raw", name], text=True).strip()


def render(name, title, metrics, start, end, stat="Average", period=60, yleft=None, extra=None):
    w = {"title": title, "metrics": metrics, "start": start, "end": end, "period": period, "stat": stat,
         "view": "timeSeries", "width": 900, "height": 380, "region": REGION, "timezone": "+0000",
         "legend": {"position": "bottom"}}
    if yleft:
        w["yAxis"] = {"left": yleft}
    if extra:
        w.update(extra)
    out = subprocess.check_output(["aws", "cloudwatch", "get-metric-widget-image", "--profile", PROFILE, "--region", REGION,
                                   "--metric-widget", json.dumps(w), "--output-format", "png", "--output", "text"])
    path = f"screenshots/{name}.png"
    with open(path, "wb") as f:
        f.write(base64.b64decode(out))
    print("wrote", path)


def main():
    kind, start, end, prefix = sys.argv[1:5]
    if kind == "dynamodb":
        t = tf_out("table_name")
        render(f"{prefix}-ddb-latency", "DynamoDB SuccessfulRequestLatency (ms): average and p99 per operation",
               [["AWS/DynamoDB", "SuccessfulRequestLatency", "TableName", t, "Operation", op, {"stat": s, "label": f"{op} {s}"}]
                for op in ("GetItem", "PutItem", "UpdateItem") for s in ("Average", "p99")], start, end)
        render(f"{prefix}-ddb-capacity", "DynamoDB consumed capacity units per minute",
               [["AWS/DynamoDB", "ConsumedWriteCapacityUnits", "TableName", t, {"label": "Write units"}],
                ["AWS/DynamoDB", "ConsumedReadCapacityUnits", "TableName", t, {"label": "Read units"}]], start, end, stat="Sum")
        render(f"{prefix}-ddb-errors", "DynamoDB throttles and errors per minute",
               [["AWS/DynamoDB", "ReadThrottleEvents", "TableName", t, {"label": "Read throttle events"}],
                ["AWS/DynamoDB", "WriteThrottleEvents", "TableName", t, {"label": "Write throttle events"}],
                ["AWS/DynamoDB", "ConditionalCheckFailedRequests", "TableName", t, {"label": "Conditional check failed"}],
                ["AWS/DynamoDB", "SystemErrors", "TableName", t, "Operation", "GetItem", {"label": "System errors GetItem"}],
                ["AWS/DynamoDB", "SystemErrors", "TableName", t, "Operation", "UpdateItem", {"label": "System errors UpdateItem"}],
                ["AWS/DynamoDB", "UserErrors", {"label": "User errors (account)"}]], start, end, stat="Sum", yleft={"min": 0})
    elif kind == "insights":
        t = tf_out("table_name")
        d = json.loads(subprocess.check_output(["aws", "dynamodb", "describe-contributor-insights", "--profile", PROFILE,
                                                "--region", REGION, "--table-name", t]))
        rules = [r for r in d.get("ContributorInsightsRuleList", []) if "PKC" in r and "Throttle" not in r]
        rule = rules[0]
        render(f"{prefix}-insights", f"Contributor Insights: most accessed partition keys ({rule})",
               [[{"expression": f"INSIGHT_RULE_METRIC('{rule}', 'UniqueContributors')", "label": "Unique cart IDs per minute", "id": "u"}],
                [{"expression": f"INSIGHT_RULE_METRIC('{rule}', 'MaxContributorValue')", "label": "Busiest single key (requests/min)", "id": "m", "yAxis": "right"}]],
               start, end)
        print("rule", rule)
    elif kind == "ecs":
        c, s = tf_out("cluster_name"), tf_out("service_name")
        render(f"{prefix}-ecs", "ECS cart-service CPU and memory (%)",
               [["AWS/ECS", "CPUUtilization", "ClusterName", c, "ServiceName", s, {"stat": "Average", "label": "CPU average"}],
                ["AWS/ECS", "CPUUtilization", "ClusterName", c, "ServiceName", s, {"stat": "Maximum", "label": "CPU max"}],
                ["AWS/ECS", "MemoryUtilization", "ClusterName", c, "ServiceName", s, {"stat": "Average", "label": "Memory average"}]],
               start, end, yleft={"min": 0, "max": 100})
    elif kind == "alb":
        lb = tf_out("alb_arn_suffix")
        render(f"{prefix}-alb", "ALB TargetResponseTime (s) and requests per minute",
               [["AWS/ApplicationELB", "TargetResponseTime", "LoadBalancer", lb, {"stat": "Average", "label": "Avg response time"}],
                ["AWS/ApplicationELB", "TargetResponseTime", "LoadBalancer", lb, {"stat": "p95", "label": "p95 response time"}],
                ["AWS/ApplicationELB", "RequestCount", "LoadBalancer", lb, {"stat": "Sum", "label": "Requests", "yAxis": "right"}]],
               start, end)
    elif kind == "rds":
        db = "hw8-mysql"
        dim = ["DBInstanceIdentifier", db]
        render(f"{prefix}-rds-cpu-conn", "RDS CPU (%) and DatabaseConnections",
               [["AWS/RDS", "CPUUtilization", *dim, {"label": "CPU %"}],
                ["AWS/RDS", "DatabaseConnections", *dim, {"label": "Connections", "yAxis": "right", "stat": "Maximum"}]], start, end)
        render(f"{prefix}-rds-iops", "RDS ReadIOPS and WriteIOPS",
               [["AWS/RDS", "ReadIOPS", *dim, {"label": "Read IOPS"}], ["AWS/RDS", "WriteIOPS", *dim, {"label": "Write IOPS"}]], start, end)
        render(f"{prefix}-rds-latency", "RDS ReadLatency and WriteLatency (s)",
               [["AWS/RDS", "ReadLatency", *dim, {"label": "Read latency"}], ["AWS/RDS", "WriteLatency", *dim, {"label": "Write latency"}]], start, end)


if __name__ == "__main__":
    main()

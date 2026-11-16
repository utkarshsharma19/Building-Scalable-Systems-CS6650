#!/usr/bin/env python3
"""Per-minute CloudWatch numbers for a test window -> JSON (min/max/mean of the per-minute values).
usage: cw_stats.py LABEL START END [rds]   (appends to results/cw_stats.json)"""
import json
import pathlib
import subprocess
import sys

P = ["--profile", "hw8", "--region", "us-west-2"]
OUT = pathlib.Path(__file__).resolve().parent.parent / "results" / "cw_stats.json"


def series(ns, metric, dims, stat, start, end):
    d = json.loads(subprocess.check_output(["aws", "cloudwatch", "get-metric-statistics", *P, "--namespace", ns, "--metric-name", metric,
                                            "--dimensions", *[f"Name={k},Value={v}" for k, v in dims], "--start-time", start,
                                            "--end-time", end, "--period", "60", "--statistics" if not stat.startswith("p") else "--extended-statistics", stat]))
    pts = sorted(d["Datapoints"], key=lambda p: p["Timestamp"])
    vals = [p.get(stat, p.get("ExtendedStatistics", {}).get(stat)) for p in pts]
    vals = [v for v in vals if v is not None]
    if not vals:
        return None
    return {"min": round(min(vals), 3), "max": round(max(vals), 3), "mean": round(sum(vals) / len(vals), 3), "points": len(vals), "sum": round(sum(vals), 3)}


def main():
    label, start, end = sys.argv[1:4]
    rds = len(sys.argv) > 4 and sys.argv[4] == "rds"
    ecs = [("ClusterName", "hw8-cluster"), ("ServiceName", "hw8-cart-service")]
    lb = subprocess.check_output(["terraform", "-chdir=terraform", "output", "-raw", "alb_arn_suffix"], text=True).strip()
    r = {
        "window": [start, end],
        "ecs_cpu_avg": series("AWS/ECS", "CPUUtilization", ecs, "Average", start, end),
        "ecs_cpu_max": series("AWS/ECS", "CPUUtilization", ecs, "Maximum", start, end),
        "ecs_mem_avg": series("AWS/ECS", "MemoryUtilization", ecs, "Average", start, end),
        "alb_target_rt_avg_s": series("AWS/ApplicationELB", "TargetResponseTime", [("LoadBalancer", lb)], "Average", start, end),
        "alb_target_rt_p95_s": series("AWS/ApplicationELB", "TargetResponseTime", [("LoadBalancer", lb)], "p95", start, end),
        "alb_requests": series("AWS/ApplicationELB", "RequestCount", [("LoadBalancer", lb)], "Sum", start, end),
    }
    if rds:
        db = [("DBInstanceIdentifier", "hw8-mysql")]
        for m, st in (("CPUUtilization", "Average"), ("CPUUtilization", "Maximum"), ("DatabaseConnections", "Maximum"),
                      ("ReadIOPS", "Average"), ("WriteIOPS", "Average"), ("ReadLatency", "Average"), ("WriteLatency", "Average"),
                      ("FreeableMemory", "Average")):
            r[f"rds_{m}_{st}"] = series("AWS/RDS", m, db, st, start, end)
    else:
        t = [("TableName", "hw8-shopping-carts")]
        r["ddb_wcu"] = series("AWS/DynamoDB", "ConsumedWriteCapacityUnits", t, "Sum", start, end)
        r["ddb_rcu"] = series("AWS/DynamoDB", "ConsumedReadCapacityUnits", t, "Sum", start, end)
        for op in ("GetItem", "PutItem", "UpdateItem"):
            r[f"ddb_latency_{op}_avg"] = series("AWS/DynamoDB", "SuccessfulRequestLatency", t + [("Operation", op)], "Average", start, end)
            r[f"ddb_latency_{op}_p99"] = series("AWS/DynamoDB", "SuccessfulRequestLatency", t + [("Operation", op)], "p99", start, end)
        r["ddb_read_throttles"] = series("AWS/DynamoDB", "ReadThrottleEvents", t, "Sum", start, end)
        r["ddb_write_throttles"] = series("AWS/DynamoDB", "WriteThrottleEvents", t, "Sum", start, end)
        r["ddb_conditional_failed"] = series("AWS/DynamoDB", "ConditionalCheckFailedRequests", t, "Sum", start, end)
    allr = json.loads(OUT.read_text()) if OUT.exists() else {}
    allr[label] = r
    OUT.write_text(json.dumps(allr, indent=2))
    print(json.dumps({k: (v if not isinstance(v, dict) else {kk: v[kk] for kk in ("min", "max", "mean", "sum")}) for k, v in r.items()}, indent=0))


if __name__ == "__main__":
    main()

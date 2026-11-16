"""Contributor Insights numbers per window -> results/insights.json"""
import json, subprocess, sys, pathlib
R = "DynamoDBContributorInsights-PKC-hw8-shopping-carts-1794818871797"
W = {"probe": ("2026-11-16T08:55:00Z", "2026-11-16T08:59:00Z"), "pool200": ("2026-11-16T09:05:00Z", "2026-11-16T09:09:00Z"),
     "pool10": ("2026-11-16T09:19:00Z", "2026-11-16T09:23:00Z")}
out = {}
for lbl, (s, e) in W.items():
    d = json.loads(subprocess.check_output(["aws", "cloudwatch", "get-insight-rule-report", "--profile", "hw8", "--rule-name", R,
        "--start-time", s, "--end-time", e, "--period", "60", "--max-contributor-count", "5",
        "--metrics", "UniqueContributors", "MaxContributorValue", "Sum"]))
    dp = d["MetricDatapoints"]
    out[lbl] = {"unique_per_min": [int(x.get("UniqueContributors", 0)) for x in dp], "busiest_key_per_min": [int(x.get("MaxContributorValue", 0)) for x in dp],
                "requests_per_min": [int(x.get("Sum", 0)) for x in dp], "approx_unique_window": d.get("ApproximateUniqueCount"),
                "top": [(c["Keys"], int(c["ApproximateAggregateValue"])) for c in d["Contributors"][:5]]}
    print(lbl, out[lbl])
pathlib.Path("results/insights.json").write_text(json.dumps(out, indent=2))

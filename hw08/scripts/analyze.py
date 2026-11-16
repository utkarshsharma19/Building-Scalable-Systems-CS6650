#!/usr/bin/env python3
"""All numbers and charts for the report come from here.

- Part 0: verify mysql_test_results.json and dynamodb_test_results.json (150 ops, 50 per op),
  merge into combined_results.json (adds a "database" field), compute every STEP III table from it.
- Locust and consistency summaries, charts in charts/.
Writes results/analysis.json (read by the report builder).
"""
import csv
import json
import pathlib
import sys

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt  # noqa: E402

R = pathlib.Path(__file__).resolve().parent.parent
RES, CH = R / "results", R / "charts"
CH.mkdir(exist_ok=True)
OPS = ["create_cart", "add_items", "get_cart"]
BLUE, ORANGE, GRAY, GREEN = "#2a78d6", "#e8833a", "#9b9a95", "#3a9e6f"
plt.rcParams.update({"font.family": "Helvetica", "font.size": 9, "axes.spines.top": False, "axes.spines.right": False})


def pct(xs, p):
    """Percentile with linear interpolation (same as numpy's default)."""
    xs = sorted(xs)
    if not xs:
        return None
    k = (len(xs) - 1) * p / 100
    f = int(k)
    c = min(f + 1, len(xs) - 1)
    return xs[f] + (xs[c] - xs[f]) * (k - f)


def stats(recs):
    rt = [r["response_time"] for r in recs]
    return {"n": len(rt), "success": sum(r["success"] for r in recs), "success_rate": round(100 * sum(r["success"] for r in recs) / len(rt), 2),
            "avg": round(sum(rt) / len(rt), 2), "p50": round(pct(rt, 50), 2), "p95": round(pct(rt, 95), 2),
            "p99": round(pct(rt, 99), 2), "min": round(min(rt), 2), "max": round(max(rt), 2)}


def verify(path):
    recs = json.loads(path.read_text())
    counts = {op: sum(r["operation"] == op for r in recs) for op in OPS}
    keys_ok = all(set(r) == {"operation", "response_time", "success", "status_code", "timestamp"} for r in recs)
    ok = len(recs) == 150 and all(c == 50 for c in counts.values()) and keys_ok
    return recs, {"file": path.name, "total": len(recs), **counts, "successful": sum(r["success"] for r in recs),
                  "failed": sum(not r["success"] for r in recs), "format_ok": keys_ok, "valid": ok}


def locust(label):
    p = RES / f"locust-{label}_stats.csv"
    if not p.exists():
        return None
    rows = {r["Name"]: r for r in csv.DictReader(p.open())}
    a = rows["Aggregated"]
    out = {"requests": int(a["Request Count"]), "failures": int(a["Failure Count"]), "rps": round(float(a["Requests/s"]), 1),
           "avg": round(float(a["Average Response Time"]), 1), "p50": int(a["50%"]), "p95": int(a["95%"]), "p99": int(a["99%"]),
           "p999": int(a["99.9%"]), "max": round(float(a["Max Response Time"]))}
    out["per_op"] = {n: {"p50": int(rows[n]["50%"]), "p95": int(rows[n]["95%"]), "p99": int(rows[n]["99%"])} for n in OPS if n in rows}
    return out


def server(name):
    p = RES / f"server-{name}.json"
    if not p.exists():
        return None
    return {r["op"]: {k: (round(float(v), 2) if k != "op" else v) for k, v in r.items()} for r in json.loads(p.read_text())}


def main():
    A = {}
    # ---------- STEP II ----------
    ddb, A["verify_dynamodb"] = verify(RES / "dynamodb_test_results.json")
    A["dynamodb"] = {"all": stats(ddb), **{op: stats([r for r in ddb if r["operation"] == op]) for op in OPS}}
    A["network_baseline"] = json.loads((RES / "network_baseline.json").read_text())
    A["server_test150_dynamodb"] = server("test150")
    for lbl in ("pool200", "pool10", "mysql"):
        A[f"locust_{lbl}"] = locust(lbl)
        A[f"server_locust_{lbl}"] = server(f"locust-{lbl}")

    c = json.loads((RES / "dynamodb_consistency_results.json").read_text())
    probes = {"create": [c["probe_create"]["results"]], "update": [c["probe_update"]["results"]]}
    for m in ("create", "update"):
        for run in (1, 2):
            p = RES / f"probe-{m}-{run}.json"
            if p.exists():
                probes[m].append(json.loads(p.read_text())["results"])
    A["consistency"] = {"client": c["summary"], "concurrent": c["concurrent_same_cart"]}
    for m, runs in probes.items():
        allr = [t for r in runs for t in r]
        stale = [t for t in allr if t["stale"]]
        conv = sorted(t["converge_ms"] for t in stale)
        A["consistency"][f"probe_{m}"] = {
            "trials": len(allr), "stale": len(stale), "stale_pct": round(100 * len(stale) / len(allr), 2),
            "per_run": [sum(t["stale"] for t in r) for r in runs],
            "converge_ms": conv, "max_converge_ms": max(conv) if conv else 0,
            "median_converge_ms": round(pct(conv, 50), 2) if conv else 0,
            "max_stale_reads": max((t["stale_reads"] for t in stale), default=0),
            "write_ms_avg": round(sum(t["write_ms"] for t in allr) / len(allr), 2),
            "read_ms_avg": round(sum(t["first_read_ms"] for t in allr) / len(allr), 2)}
    mc = RES / "mysql_consistency_results.json"
    if mc.exists():
        m = json.loads(mc.read_text())
        A["consistency_mysql"] = {"client": m["summary"], "concurrent": m["concurrent_same_cart"]}

    # ---------- STEP III (only if MySQL ran) ----------
    mp = RES / "mysql_test_results.json"
    if mp.exists():
        my, A["verify_mysql"] = verify(mp)
        A["mysql"] = {"all": stats(my), **{op: stats([r for r in my if r["operation"] == op]) for op in OPS}}
        A["server_test150_mysql"] = server("test150-mysql")
        combined = [{**r, "database": "mysql"} for r in my] + [{**r, "database": "dynamodb"} for r in ddb]
        (RES / "combined_results.json").write_text(json.dumps(combined, indent=2))
        # recompute everything from combined_results.json only
        cb = json.loads((RES / "combined_results.json").read_text())
        S = {db: {"all": stats([r for r in cb if r["database"] == db]),
                  **{op: stats([r for r in cb if r["database"] == db and r["operation"] == op]) for op in OPS}}
             for db in ("mysql", "dynamodb")}
        A["combined"] = {"records": len(cb), "by_db_op": {db: {op: sum(r["database"] == db and r["operation"] == op for r in cb) for op in OPS}
                                                          for db in ("mysql", "dynamodb")}, "stats": S}
        table = []
        for metric, key in (("Avg Response Time (ms)", "avg"), ("P50 Response Time (ms)", "p50"), ("P95 Response Time (ms)", "p95"),
                            ("P99 Response Time (ms)", "p99"), ("Success Rate (%)", "success_rate")):
            m_, d_ = S["mysql"]["all"][key], S["dynamodb"]["all"][key]
            if key == "success_rate":
                win = "Tie" if m_ == d_ else ("MySQL" if m_ > d_ else "DynamoDB")
            else:
                win = "Tie" if m_ == d_ else ("MySQL" if m_ < d_ else "DynamoDB")
            diff = abs(m_ - d_)
            margin = "0" if diff == 0 else f"{diff:.2f}" + ("" if key == "success_rate" else f" ms ({100 * diff / max(m_, d_):.1f}%)")
            table.append([metric, m_, d_, win, margin])
        A["table_overall"] = table
        A["table_ops"] = [[op.upper(), S["mysql"][op]["avg"], S["dynamodb"][op]["avg"],
                           ("MySQL" if S["mysql"][op]["avg"] < S["dynamodb"][op]["avg"] else "DynamoDB") + f", {abs(S['mysql'][op]['avg'] - S['dynamodb'][op]['avg']):.2f} ms"]
                          for op in OPS]
        charts_step3(S, A)
    charts_step2(A)
    (RES / "analysis.json").write_text(json.dumps(A, indent=2))
    print(json.dumps({k: A[k] for k in A if k in ("verify_dynamodb", "verify_mysql", "table_overall", "table_ops")}, indent=1))


def charts_step2(A):
    # client time split into network baseline and the rest
    base = A["network_baseline"]["avg_ms"]
    sv = A["server_test150_dynamodb"]
    fig, ax = plt.subplots(figsize=(6.4, 2.6))
    labels = OPS
    client = [A["dynamodb"][op]["avg"] for op in OPS]
    svc = [sv[op]["svc_avg"] for op in OPS]
    ax.barh(labels, [base] * 3, color=GRAY, label=f"network to us-west-2 (/health, {base:.1f} ms)")
    ax.barh(labels, svc, left=[base] * 3, color=BLUE, label="in the service (request logs)")
    ax.barh(labels, [c - base - s for c, s in zip(client, svc)], left=[base + s for s in svc], color="#c9c8c2", label="rest (ALB, client)")
    for i, cval in enumerate(client):
        ax.text(cval + 1, i, f"{cval:.1f} ms", va="center", fontsize=8)
    ax.set_xlabel("average response time from my laptop (ms)")
    ax.set_xlim(0, max(client) * 1.18)
    ax.legend(fontsize=7, loc="lower center", bbox_to_anchor=(0.5, -0.62), ncol=3, frameon=False)
    fig.tight_layout()
    fig.savefig(CH / "ddb-150-breakdown.png", dpi=200, bbox_inches="tight")
    plt.close(fig)

    # pool 10 vs pool 200 vs Eason
    runs = [("Eason, SDK default\n(laptop RTT ~35 ms)", {"p50": 39, "p95": 170, "p99": 290}, GRAY)]
    if A.get("locust_pool10"):
        runs.append(("Mine, pool 10\n(SDK default)", A["locust_pool10"], ORANGE))
    if A.get("locust_pool200"):
        runs.append(("Mine, pool 200", A["locust_pool200"], BLUE))
    fig, ax = plt.subplots(figsize=(6.4, 2.7))
    w = 0.25
    for i, (lbl, d, col) in enumerate(runs):
        vals = [d["p50"], d["p95"], d["p99"]]
        xs = [j + (i - (len(runs) - 1) / 2) * w for j in range(3)]
        ax.bar(xs, vals, w, color=col, label=lbl.replace("\n", " "))
        for x, v in zip(xs, vals):
            ax.text(x, v + 3, str(v), ha="center", fontsize=7)
    ax.set_xticks(range(3), ["p50", "p95", "p99"])
    ax.set_ylabel("Locust response time (ms)")
    ax.legend(fontsize=7, frameon=False)
    fig.tight_layout()
    fig.savefig(CH / "ddb-locust-pools.png", dpi=200, bbox_inches="tight")
    plt.close(fig)


def charts_step3(S, A):
    fig, axes = plt.subplots(1, 2, figsize=(6.6, 2.5))
    w = 0.38
    for ax, keys, title in ((axes[0], OPS, "average per operation"), (axes[1], ["p50", "p95", "p99"], "overall percentiles")):
        if keys is OPS:
            m = [S["mysql"][op]["avg"] for op in OPS]; d = [S["dynamodb"][op]["avg"] for op in OPS]; lab = ["create", "add", "get"]
        else:
            m = [S["mysql"]["all"][k] for k in keys]; d = [S["dynamodb"]["all"][k] for k in keys]; lab = keys
        ax.bar([i - w / 2 for i in range(3)], m, w, color=ORANGE, label="MySQL")
        ax.bar([i + w / 2 for i in range(3)], d, w, color=BLUE, label="DynamoDB")
        ax.set_xticks(range(3), lab)
        ax.set_title(title, fontsize=9)
        lo = min(m + d)
        ax.set_ylim(max(0, lo - 15), max(m + d) * 1.05)
    axes[0].set_ylabel("ms (from combined_results.json)")
    axes[1].legend(fontsize=7, frameon=False)
    fig.tight_layout()
    fig.savefig(CH / "step3-comparison.png", dpi=200, bbox_inches="tight")
    plt.close(fig)


if __name__ == "__main__":
    sys.exit(main())

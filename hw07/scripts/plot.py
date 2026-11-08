"""Charts for the HW7 report, from results/*. Usage: .venv/bin/python scripts/plot.py"""
import csv
import json
import statistics
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt

ROOT = Path(__file__).resolve().parent.parent
RES, OUT = ROOT / "results", ROOT / "charts"
OUT.mkdir(exist_ok=True)

# Reference categorical palette (fixed order), recessive ink for text/axes.
S1, S2, S3, S4 = "#2a78d6", "#eb6834", "#1baf7a", "#eda100"
INK, INK2, GRID = "#0b0b0b", "#52514e", "#e4e3df"
plt.rcParams.update({
    "font.size": 10, "axes.edgecolor": INK2, "axes.labelcolor": INK2, "xtick.color": INK2,
    "ytick.color": INK2, "axes.titlecolor": INK, "axes.titleweight": "bold", "axes.titlesize": 11,
    "axes.spines.top": False, "axes.spines.right": False, "axes.grid": True, "grid.color": GRID,
    "grid.linewidth": 0.8, "axes.axisbelow": True, "figure.dpi": 150, "savefig.bbox": "tight",
    "lines.linewidth": 2, "legend.frameon": False,
})


def summary(name):
    for row in csv.DictReader(open(RES / name / "locust_stats.csv")):
        if row["Name"] == "Aggregated":
            return row
    return None


def history(name):
    rows = [r for r in csv.DictReader(open(RES / name / "locust_stats_history.csv"))
            if r["Name"] == "Aggregated"]
    t0 = int(rows[0]["Timestamp"])
    return t0, rows


def queue(name):
    p = RES / name / "queue.csv"
    if not p.exists():
        return None
    return [(int(r["epoch"]), int(r["visible"])) for r in csv.DictReader(open(p))]


def processor_rate(name):
    """Mean completed/s over 10 s windows where every worker slot stayed busy."""
    p = RES / name / "processor-stats.log"
    if not p.exists():
        return None
    rates = []
    for line in open(p):
        try:
            s = json.loads(line.split(" ", 1)[1])
        except (IndexError, json.JSONDecodeError):
            continue
        if s.get("in_flight", 0) >= s["workers"] * 0.9 and s["rate_per_s"] > 0:
            rates.append(s["rate_per_s"])
    return statistics.mean(rates) if rates else None


def save(fig, name):
    fig.savefig(OUT / name)
    plt.close(fig)
    print("wrote", OUT / name)


# 1. Throughput: sync vs async under the same flash-sale load
tests = [("p1-sync-normal", "Sync, normal\n5 users"), ("p1-sync-flash", "Sync, flash\n20 users"),
         ("p3-async-w1", "Async, flash\n20 users")]
tests = [(n, l) for n, l in tests if (RES / n / "locust_stats.csv").exists()]
if tests:
    fig, ax = plt.subplots(figsize=(6, 3.4))
    vals = [float(summary(n)["Requests/s"]) for n, _ in tests]
    bars = ax.bar([l for _, l in tests], vals, color=[S2, S2, S1][: len(tests)], width=0.55)
    for b, v in zip(bars, vals):
        ax.annotate(f"{v:.2f}/s" if v < 1 else f"{v:.1f}/s", (b.get_x() + b.get_width() / 2, v), ha="center", va="bottom",
                    xytext=(0, 3), textcoords="offset points", color=INK)
    ax.set_ylabel("Orders accepted per second")
    ax.set_title(f"Async accepted {vals[-1] / vals[-2]:.0f}x more orders per second than sync")
    save(fig, "1-throughput-sync-vs-async.png")

    # 2. Latency (log scale): median and p95
    fig, ax = plt.subplots(figsize=(6, 3.4))
    x = range(len(tests))
    p50 = [float(summary(n)["50%"]) / 1000 for n, _ in tests]
    p95 = [float(summary(n)["95%"]) / 1000 for n, _ in tests]
    ax.bar([i - 0.17 for i in x], p50, 0.32, color=S1, label="median")
    ax.bar([i + 0.17 for i in x], p95, 0.32, color=S2, label="p95")
    for i in x:
        ax.annotate(f"{p50[i]:g}s", (i - 0.17, p50[i]), ha="center", va="bottom", fontsize=8, color=INK2)
        ax.annotate(f"{p95[i]:g}s", (i + 0.17, p95[i]), ha="center", va="bottom", fontsize=8, color=INK2)
    ax.set_yscale("log")
    ax.set_xticks(list(x), [l for _, l in tests])
    ax.set_ylabel("Response time (s, log scale)")
    ax.set_title("Sync waits up to a minute; async replies in ~0.1 s")
    ax.legend()
    save(fig, "2-latency-sync-vs-async.png")

# 3. Sync flash sale: latency climbs toward the ALB timeout
if (RES / "p1-sync-flash" / "locust_stats_history.csv").exists():
    t0, rows = history("p1-sync-flash")
    pts = [((int(r["Timestamp"]) - t0), float(r["50%"]) / 1000, float(r["95%"]) / 1000)
           for r in rows if r["50%"] not in ("N/A", "0", "")]
    fig, ax = plt.subplots(figsize=(6.5, 3.4))
    ax.plot([p[0] for p in pts], [p[1] for p in pts], color=S1, label="median")
    ax.plot([p[0] for p in pts], [p[2] for p in pts], color=S2, label="p95")
    ax.axhline(60, color=INK2, linestyle="--", linewidth=1)
    ax.text(1, 61, "ALB idle timeout (60 s): requests past this get 504", fontsize=8, color=INK2)
    ax.set_xlabel("Seconds into the flash sale")
    ax.set_ylabel("Response time (s)")
    ax.set_ylim(0, 70)
    ax.set_title("Sync flash sale: every new customer waits longer")
    ax.legend(loc="center right")
    save(fig, "3-sync-flash-latency-over-time.png")

# 4. Queue depth over time for each worker count
runs = [("p3-async-w1", "1 worker", S2), ("p5-w5", "5 workers", S4), ("p5-w20", "20 workers", S3),
        ("p5-w100", "100 workers", S1)]
fig, ax = plt.subplots(figsize=(7, 3.8))
plotted = False
for name, label, color in runs:
    q = queue(name)
    if not q or not (RES / name / "locust_stats_history.csv").exists():
        continue
    t0, _ = history(name)
    pts = [((e - t0) / 60, v) for e, v in q if e >= t0 - 10]
    peak = max(p[1] for p in pts)
    ax.plot([p[0] for p in pts], [p[1] for p in pts], color=color, label=f"{label} (peak {peak:,})")
    if name == "p3-async-w1":
        ax.annotate("queue purged\n(would take ~2.4 h)", pts[-1], xytext=(6, 0), textcoords="offset points",
                    fontsize=8, color=INK2, va="center")
    plotted = True
if plotted:
    ax.axvspan(0, 1, color=GRID, alpha=0.6, linewidth=0)
    ax.text(0.5, -150, "sale", ha="center", va="top", fontsize=8, color=INK2)
    ax.set_ylim(-300, None)
    ax.set_xlabel("Minutes since the flash sale started")
    ax.set_ylabel("Messages waiting in SQS")
    ax.set_title("Queue depth: more workers, smaller and shorter backlog")
    ax.legend()
    save(fig, "4-queue-depth-by-workers.png")
else:
    plt.close(fig)

# 5. Processing rate vs workers: measured vs N/3 theory vs 60/s demand
meas = []
for n, name in [(1, "p3-async-w1"), (5, "p5-w5"), (20, "p5-w20"), (100, "p5-w100")]:
    r = processor_rate(name)
    if r:
        meas.append((n, r))
if meas:
    fig, ax = plt.subplots(figsize=(6.5, 3.6))
    xs = [1, 5, 20, 100, 180]
    ticks = [0, 20, 50, 100, 150, 180]
    ax.plot(xs, [x / 3 for x in xs], color=INK2, linestyle=":", linewidth=1.5, label="theory: N / 3 s")
    ax.plot([m[0] for m in meas], [m[1] for m in meas], "o", color=S1, markersize=8, label="measured")
    key = "Measured (theory)\n" + "\n".join(f"{n:>3} workers: {r:5.2f}/s ({n / 3:5.2f})" for n, r in meas)
    ax.text(0.03, 0.62, key, transform=ax.transAxes, fontsize=8, color=INK, va="top", family="monospace")
    ax.axhline(60, color=S2, linewidth=1.5)
    ax.text(2, 62, "flash-sale demand ~60 orders/s -> needs 180 workers", fontsize=8, color=INK2)
    ax.set_xticks(ticks)
    ax.set_xlabel("Worker goroutines")
    ax.set_ylabel("Orders processed per second")
    ax.set_title("Processing rate grows linearly with workers")
    ax.legend(loc="upper left", bbox_to_anchor=(0, 0.92))
    save(fig, "5-processing-rate-vs-workers.png")

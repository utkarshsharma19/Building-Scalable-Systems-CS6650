"""Builds the charts in charts/ from the CSV/JSON files in results/.
usage: python scripts/plot.py
"""
import json
import pathlib

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import pandas as pd

PART2 = pathlib.Path(__file__).resolve().parent.parent
RES, OUT = PART2 / "results", PART2 / "charts"
OUT.mkdir(exist_ok=True)
SERIES = ["#2a78d6", "#eb6834", "#1baf7a", "#6250d6", "#e34948"]
INK, MUTED, GRID = "#0b0b0b", "#52514e", "#e4e3df"
plt.rcParams.update({
    "font.size": 11, "axes.edgecolor": GRID, "axes.labelcolor": MUTED, "xtick.color": MUTED,
    "ytick.color": MUTED, "axes.grid": True, "grid.color": GRID, "grid.linewidth": 0.8,
    "axes.spines.top": False, "axes.spines.right": False, "lines.linewidth": 2,
    "figure.facecolor": "#fcfcfb", "axes.facecolor": "#fcfcfb", "axes.titlecolor": INK,
    "axes.titleweight": "bold", "legend.frameon": False,
})
SIZES = {"f1k.bin": "1 KB", "f16k.bin": "16 KB", "f64k.bin": "64 KB", "f256k.bin": "256 KB", "f1m.bin": "1 MB"}


def exp1():
    path = RES / "exp1_loss_sweep.csv"
    if not path.exists():
        return
    df = pd.read_csv(path)
    df["ok"] = df["sha_ok"].astype(str) == "True"
    order = df.drop_duplicates("label")[["label", "send_loss", "recv_loss"]]
    files = [f for f in SIZES if f in set(df.file)]
    x = range(len(files))
    fig, (a1, a2) = plt.subplots(1, 2, figsize=(12, 4.6))
    for i, row in enumerate(order.itertuples()):
        sub = df[df.label == row.label]
        rate = [100 * sub[sub.file == f].ok.mean() for f in files]
        med = [sub[(sub.file == f)].latency_s.median() for f in files]
        name = f"send {row.send_loss}% / recv {row.recv_loss}%"
        a1.plot(x, rate, marker="o", ms=8, color=SERIES[i % 5], label=name)
        a2.plot(x, med, marker="o", ms=8, color=SERIES[i % 5], label=name)
    for a in (a1, a2):
        a.set_xticks(list(x), [SIZES[f] for f in files])
        a.set_xlabel("file size (64-byte chunks: 16 ... 16,384)")
    a1.set_ylim(-5, 105)
    a1.set_ylabel("successful downloads (%)")
    a1.set_title("Success rate by file size and loss")
    a2.set_ylabel("median latency (s)")
    a2.set_title("Median latency (every fetch waits >= 2 s)")
    a2.axhline(2, color=MUTED, lw=1, ls="--")
    a1.legend(loc="lower left")
    fig.tight_layout()
    fig.savefig(OUT / "exp1_loss_vs_size.png", dpi=160)
    plt.close(fig)

    # retry volume from the gateway's FETCH_STATS lines
    rows = []
    for row in order.itertuples():
        p = RES / f"{row.label}_gateway_fetch_stats.csv"
        if p.exists():
            s = pd.read_csv(p)
            s["label"] = f"send {row.send_loss}% / recv {row.recv_loss}%"
            rows.append(s)
    if rows:
        s = pd.concat(rows)
        s = s.groupby(["label", "file"], as_index=False).apply(
            lambda g: pd.Series({"retys": (g.avg_retys_sent * g.fetches).sum() / g.fetches.sum()}),
            include_groups=False)
        fig, ax = plt.subplots(figsize=(7.5, 4.4))
        for i, lab in enumerate(s.label.unique()):
            sub = s[s.label == lab].set_index("file").reindex(files)
            ax.plot(x, sub.retys, marker="o", ms=8, color=SERIES[i % 5], label=lab)
        ax.axhline(32, color=SERIES[4], lw=1, ls="--")
        ax.text(0, 40, "backend job queue = 32 slots per task", color=MUTED, fontsize=9)
        ax.set_yscale("symlog", linthresh=10)
        ax.set_ylim(bottom=0)
        ax.set_xticks(list(x), [SIZES[f] for f in files])
        ax.set_ylabel("RETY datagrams sent per fetch (avg)")
        ax.set_title("Retry volume per download")
        ax.legend(loc="upper left", fontsize=9)
        fig.tight_layout()
        fig.savefig(OUT / "exp1_retry_volume.png", dpi=160)
        plt.close(fig)


def exp2():
    path = RES / "exp2_runs.csv"
    if not path.exists():
        return
    runs = pd.read_csv(path, names=["label", "inflight", "users", "start", "end"]).drop_duplicates("label", keep="last")
    rec = []
    for r in runs.itertuples():
        st = pd.read_csv(RES / f"{r.label}_stats.csv")
        agg = st[st.Name == "Aggregated"].iloc[0]
        fails = RES / f"{r.label}_failures.csv"
        f = pd.read_csv(fails) if fails.exists() else pd.DataFrame(columns=["Error", "Occurrences"])
        n = agg["Request Count"]
        ok = n - agg["Failure Count"]
        rec.append({
            "inflight": r.inflight, "users": r.users, "ok_rps": ok / (r.end - r.start),
            "p50": agg["50%"] / 1000, "p95": agg["95%"] / 1000,
            "pct503": 100 * f[f.Error.astype(str).str.contains("503")].Occurrences.sum() / n,
            "pct504": 100 * f[f.Error.astype(str).str.contains("504")].Occurrences.sum() / n,
        })
    d = pd.DataFrame(rec).sort_values(["inflight", "users"])
    d.to_csv(RES / "exp2_summary.csv", index=False)
    fig, axes = plt.subplots(1, 3, figsize=(14, 4.4))
    for i, inf in enumerate(sorted(d.inflight.unique())):
        sub = d[d.inflight == inf]
        lab = f"MAX_INFLIGHT={inf}"
        axes[0].plot(sub.users, sub.ok_rps, marker="o", ms=8, color=SERIES[i], label=lab)
        axes[1].plot(sub.users, sub.p95, marker="o", ms=8, color=SERIES[i], label=lab)
        axes[2].plot(sub.users, sub.pct503, marker="o", ms=8, color=SERIES[i], label=f"{lab}: 503 (gateway busy)")
        axes[2].plot(sub.users, sub.pct504, marker="s", ms=8, ls="--", color=SERIES[i], label=f"{lab}: 504 (backend gave up)")
    axes[0].set_title("Successful downloads / s")
    axes[1].set_title("p95 latency (s)")
    axes[2].set_title("Failed requests (%)")
    for a in axes:
        a.set_xlabel("concurrent Locust users")
    axes[0].legend()
    axes[2].legend(fontsize=8)
    fig.tight_layout()
    fig.savefig(OUT / "exp2_inflight.png", dpi=160)
    plt.close(fig)


def exp3():
    tl, hist = RES / "exp3_scale_timeline.csv", RES / "exp3_stats_history.csv"
    if not tl.exists() or not hist.exists():
        return
    t = pd.read_csv(tl)
    h = pd.read_csv(hist)
    h = h[h.Name == "Aggregated"]
    t0 = h.Timestamp.min()
    cw = json.loads((RES / "exp3_cloudwatch.json").read_text()) if (RES / "exp3_cloudwatch.json").exists() else None
    fig, axes = plt.subplots(4, 1, figsize=(11, 11), sharex=True)
    m = lambda s: (s - t0) / 60
    axes[0].plot(m(h.Timestamp), h["User Count"], color=MUTED, label="users")
    axes[0].plot(m(h.Timestamp), h["Requests/s"], color=SERIES[0], label="requests/s")
    axes[0].plot(m(h.Timestamp), h["Failures/s"], color=SERIES[4], label="failures/s")
    axes[0].set_title("Offered load (Locust)")
    axes[0].legend(loc="upper right")
    axes[1].step(m(t.epoch), t.gw_running, where="post", color=SERIES[0], label="gateway running")
    axes[1].step(m(t.epoch), t.gw_desired, where="post", color=SERIES[0], ls="--", lw=1.2, label="gateway desired")
    axes[1].step(m(t.epoch), t.be_running, where="post", color=SERIES[1], label="backend running")
    axes[1].step(m(t.epoch), t.be_desired, where="post", color=SERIES[1], ls="--", lw=1.2, label="backend desired")
    acts = RES / "exp3_scaling_activities.json"
    if acts.exists():
        for a in json.loads(acts.read_text()):
            ts = pd.Timestamp(a["t"]).timestamp()
            if a["res"].endswith(("/gateway", "/backend")) and t0 <= ts <= t0 + 3600:
                x = (ts - t0) / 60
                for ax in axes:
                    ax.axvline(x, color=MUTED, lw=0.8, ls=":")
                axes[1].text(x - (3.2 if x > 20 else -0.2), 4.55, a["res"].split("/")[-1] + " -> " + a["desc"].split()[-1].rstrip("."),
                             fontsize=8, color=MUTED)
    axes[1].set_ylim(0, 5)
    axes[1].set_title("ECS task counts")
    axes[1].legend(loc="center right", fontsize=9)
    if cw:
        series = {r["Id"]: pd.Series(r["Values"], index=pd.to_datetime(r["Timestamps"])).sort_index()
                  for r in cw["MetricDataResults"]}
        mins = lambda s: [(ts.timestamp() - t0) / 60 for ts in s.index]
        rpt = series.get("req_per_target")
        if rpt is not None:
            axes[2].plot(mins(rpt), rpt.values, color=SERIES[0], marker="o", ms=4, label="ALB requests / target / min")
            axes[2].axhline(100, color=SERIES[0], ls="--", lw=1)
            axes[2].text(0.2, 110, "gateway target = 100", color=MUTED, fontsize=9)
        axes[2].set_title("Gateway scaling metric (ALBRequestCountPerTarget)")
        for k, c, lab in (("gw_cpu", SERIES[0], "gateway CPU %"), ("be_cpu", SERIES[1], "backend CPU %")):
            s = series.get(k)
            if s is not None:
                axes[3].plot(mins(s), s.values, color=c, marker="o", ms=4, label=lab)
        axes[3].axhline(60, color=SERIES[1], ls="--", lw=1)
        axes[3].text(0.2, 63, "backend target = 60%", color=MUTED, fontsize=9)
        axes[3].set_title("CPU utilisation (backend scaling metric)")
        axes[3].legend(loc="upper right")
    axes[3].set_xlabel("minutes since test start")
    fig.tight_layout()
    fig.savefig(OUT / "exp3_autoscale_timeline.png", dpi=160)
    plt.close(fig)


if __name__ == "__main__":
    exp1()
    exp2()
    exp3()
    print("charts in", OUT, [p.name for p in OUT.glob("*.png")])

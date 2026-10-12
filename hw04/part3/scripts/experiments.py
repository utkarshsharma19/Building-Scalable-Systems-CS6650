import json
import statistics
import subprocess
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt

from mapreduce import ENDPOINTS, run_job

ROOT = Path(__file__).parent.parent
RESULTS = ROOT / "results"
SIZES_MB = [1, 5, 10, 25, 50]
REPS = 3
BUCKET = ENDPOINTS["bucket"]


def s3(mb):
    return f"s3://{BUCKET}/input/text_{mb}MB.txt"


def median_run(url, n):
    runs = [run_job(url, n) for _ in range(REPS)]
    return {k: statistics.median(r[k] for r in runs) for k in ("split_s", "map_s", "reduce_s", "total_s")}, runs


def local_ms(mb):
    out = subprocess.run(["go", "run", "./cmd/localcount", "-in", f"data/text_{mb}MB.txt"],
                         cwd=ROOT, capture_output=True, text=True, check=True).stdout
    return json.loads(out)["count_ms"]


def scaling():
    rows = []
    for mb in SIZES_MB:
        for n in (1, 3):
            med, _ = median_run(s3(mb), n)
            rows.append({"mb": mb, "n": n, **med})
            print(f"{mb:>3}MB n={n}  total {med['total_s']:.2f}s")
    local = {mb: local_ms(mb) / 1000 for mb in SIZES_MB}

    fig, ax = plt.subplots(1, 2, figsize=(12, 4.5))
    for n, style in ((1, "o-"), (3, "s-")):
        pts = [r for r in rows if r["n"] == n]
        ax[0].plot([r["mb"] for r in pts], [r["total_s"] for r in pts], style, label=f"{n} mapper(s)")
    ax[0].plot(SIZES_MB, [local[mb] for mb in SIZES_MB], "^--", label="local count only (laptop)")
    ax[0].set(xlabel="file size (MB)", ylabel="end-to-end time (s)", title="Runtime vs file size")
    ax[0].legend()
    ax[0].grid(alpha=0.3)

    one = {r["mb"]: r["total_s"] for r in rows if r["n"] == 1}
    three = {r["mb"]: r["total_s"] for r in rows if r["n"] == 3}
    speedup = [one[mb] / three[mb] for mb in SIZES_MB]
    ax[1].bar([str(mb) for mb in SIZES_MB], speedup)
    ax[1].axhline(3, ls="--", c="gray", label="ideal (3x)")
    ax[1].axhline(1, ls=":", c="red", label="no speedup")
    for i, s in enumerate(speedup):
        ax[1].text(i, s, f"{s:.2f}x", ha="center", va="bottom")
    ax[1].set(xlabel="file size (MB)", ylabel="speedup (1 mapper / 3 mappers)", title="Speedup with 3 mappers")
    ax[1].legend()
    fig.tight_layout()
    fig.savefig(RESULTS / "scaling.png", dpi=150)

    fig, ax = plt.subplots(figsize=(8, 4.5))
    labels = [f"{r['mb']}MB\nn={r['n']}" for r in rows]
    bottom = [0] * len(rows)
    for stage in ("split_s", "map_s", "reduce_s"):
        vals = [r[stage] for r in rows]
        ax.bar(labels, vals, bottom=bottom, label=stage.removesuffix("_s"))
        bottom = [b + v for b, v in zip(bottom, vals)]
    ax.set(ylabel="time (s)", title="Where the time goes (median of 3 runs)")
    ax.legend()
    fig.tight_layout()
    fig.savefig(RESULTS / "stages.png", dpi=150)
    return {"rows": rows, "local_s": local}


def load_balance(mb=50):
    _, runs = median_run(s3(mb), 3)
    mapped = runs[-1]["mapped"]
    labels = [f"mapper {i}" for i in range(len(mapped))]

    fig, ax = plt.subplots(1, 3, figsize=(13, 4))
    for a, key, title in zip(ax, ("chunk_bytes", "total_words", "total_ms"),
                             ("chunk size (bytes)", "words counted", "mapper time (ms)")):
        vals = [m[key] for m in mapped]
        a.bar(labels, vals)
        a.set_title(title)
        spread = (max(vals) - min(vals)) / statistics.mean(vals) * 100
        a.set_xlabel(f"max-min spread: {spread:.2f}%")
    fig.suptitle(f"Load balance across 3 mappers ({mb}MB file)")
    fig.tight_layout()
    fig.savefig(RESULTS / "load_balance.png", dpi=150)
    return mapped


def chunk_count(mb=50, ns=(1, 2, 3, 6, 9)):
    rows = []
    for n in ns:
        med, _ = median_run(s3(mb), n)
        rows.append({"n": n, **med})
        print(f"{mb}MB chunks={n}  total {med['total_s']:.2f}s")

    fig, ax = plt.subplots(figsize=(7, 4.5))
    ax.plot([r["n"] for r in rows], [r["total_s"] for r in rows], "o-", label="total")
    ax.plot([r["n"] for r in rows], [r["map_s"] for r in rows], "s--", label="map stage")
    ax.axvline(3, ls=":", c="gray", label="number of mapper tasks")
    ax.set(xlabel="number of chunks", ylabel="time (s)", title=f"Chunks vs runtime ({mb}MB, 3 mappers)")
    ax.legend()
    ax.grid(alpha=0.3)
    fig.tight_layout()
    fig.savefig(RESULTS / "chunks.png", dpi=150)
    return rows


if __name__ == "__main__":
    RESULTS.mkdir(exist_ok=True)
    results = {"scaling": scaling(), "load_balance": load_balance(), "chunks": chunk_count()}
    (RESULTS / "experiments.json").write_text(json.dumps(results, indent=2))
    print("plots written to results/")

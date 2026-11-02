"""Experiment 1: reliability vs. simulated datagram loss and file size.

One request at a time (no concurrency) so that the only thing that changes between runs is
the loss setting the backend was deployed with. Each response is checked against the SHA-256
of the original file, so "200 but corrupted" would show up too.

usage: python exp1_loss_sweep.py --label send5 --send-loss 5 --recv-loss 0 [--reps 8]
Appends rows to results/exp1_loss_sweep.csv.
"""
import argparse
import csv
import hashlib
import os
import pathlib
import time

import requests

PART2 = pathlib.Path(__file__).resolve().parent.parent
FILES = ["f1k.bin", "f16k.bin", "f64k.bin", "f256k.bin", "f1m.bin"]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--label", required=True)
    ap.add_argument("--send-loss", type=int, required=True)
    ap.add_argument("--recv-loss", type=int, required=True)
    ap.add_argument("--reps", type=int, default=8)
    ap.add_argument("--files", nargs="*", default=FILES)
    ap.add_argument("--gateway", default=os.environ.get("GATEWAY"))
    ap.add_argument("--out", default=str(PART2 / "results" / "exp1_loss_sweep.csv"))
    args = ap.parse_args()
    assert args.gateway, "source scripts/env.sh first (GATEWAY unset)"

    sums = {}
    for line in (PART2 / "testfiles" / "SHA256SUMS").read_text().splitlines():
        digest, name = line.split()
        sums[name] = digest

    out = pathlib.Path(args.out)
    new = not out.exists()
    with out.open("a", newline="") as fh:
        w = csv.writer(fh)
        if new:
            w.writerow(["label", "send_loss", "recv_loss", "file", "size", "rep", "status",
                        "latency_s", "bytes", "sha_ok", "ts"])
        for name in args.files:
            size = (PART2 / "testfiles" / name).stat().st_size
            for rep in range(args.reps):
                t0 = time.time()
                try:
                    r = requests.get(f"{args.gateway}/files", params={"name": name}, timeout=180)
                    status, body = r.status_code, r.content
                except requests.RequestException as exc:
                    status, body = f"EXC:{type(exc).__name__}", b""
                dt = time.time() - t0
                ok = status == 200 and hashlib.sha256(body).hexdigest() == sums[name]
                w.writerow([args.label, args.send_loss, args.recv_loss, name, size, rep, status,
                            f"{dt:.3f}", len(body), ok, int(t0)])
                fh.flush()
                print(f"{args.label:8} {name:10} rep={rep} status={status} {dt:7.2f}s sha_ok={ok}")


if __name__ == "__main__":
    main()

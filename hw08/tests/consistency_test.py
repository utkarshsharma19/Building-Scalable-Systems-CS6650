#!/usr/bin/env python3
"""HW8 Step II Part 3: eventual-consistency investigation.

Client-side scenarios (through the ALB, what a real user/browser would experience):
  1. create cart -> immediately GET it (eventually consistent read)        x N
  2. add item    -> immediately GET the cart, check the new quantity shows  x N
  3. rapid concurrent updates: C clients x K adds to the SAME cart while readers poll;
     check no lost updates (final quantity == C*K) and whether readers ever see the
     version go backwards (a stale replica answering after a fresher one did).
Server-side probe (inside us-west-2, read issued microseconds after the write ack):
  4. POST /debug/consistency-probe mode=create and mode=update            x M

usage: consistency_test.py BASE_URL OUTPUT.json [N] [M]
"""
import http.client
import json
import sys
import threading
import time
from urllib.parse import urlparse


class Client:
    def __init__(self, base):
        u = urlparse(base)
        self.host, self.port = u.hostname, u.port
        self.conn = http.client.HTTPConnection(self.host, self.port, timeout=60)

    def req(self, method, path, body=None):
        payload = json.dumps(body) if body is not None else None
        for attempt in range(2):
            try:
                self.conn.request(method, path, body=payload, headers={"Content-Type": "application/json"} if payload else {})
                r = self.conn.getresponse()
                data = r.read()
                return r.status, (json.loads(data) if data else None)
            except (http.client.HTTPException, OSError):
                self.conn.close()
                self.conn = http.client.HTTPConnection(self.host, self.port, timeout=60)
        raise RuntimeError("request failed twice")


def read_until(c, path, fresh, limit_s=5.0):
    """Poll with eventually consistent reads until fresh(status, body). Returns (stale_reads, ms)."""
    t0 = time.perf_counter()
    stale = 0
    while True:
        st, body = c.req("GET", path)
        if fresh(st, body):
            return stale, (time.perf_counter() - t0) * 1000
        stale += 1
        if time.perf_counter() - t0 > limit_s:
            return stale, -1


def scenario_create_read(base, n):
    c = Client(base)
    trials = []
    for i in range(n):
        t0 = time.perf_counter()
        st, body = c.req("POST", "/shopping-carts", {"customer_id": 5000 + i})
        write_ms = (time.perf_counter() - t0) * 1000
        cid = body["shopping_cart_id"]
        stale, conv = read_until(c, f"/shopping-carts/{cid}", lambda s, b: s == 200)
        trials.append({"cart_id": cid, "write_ms": round(write_ms, 2), "stale_reads": stale, "converge_ms": round(conv, 2)})
    return trials


def scenario_add_read(base, n):
    c = Client(base)
    _, body = c.req("POST", "/shopping-carts", {"customer_id": 6000})
    cid = body["shopping_cart_id"]
    trials, expected = [], 0
    for i in range(n):
        expected += 1
        t0 = time.perf_counter()
        st, _ = c.req("POST", f"/shopping-carts/{cid}/items", {"product_id": 1, "quantity": 1})
        write_ms = (time.perf_counter() - t0) * 1000
        want = expected
        stale, conv = read_until(c, f"/shopping-carts/{cid}",
                                 lambda s, b: s == 200 and any(it["product_id"] == 1 and it["quantity"] >= want for it in b["items"]))
        trials.append({"write_ms": round(write_ms, 2), "stale_reads": stale, "converge_ms": round(conv, 2)})
    return {"cart_id": cid, "trials": trials}


def scenario_concurrent(base, writers=10, adds=20, readers=4):
    c = Client(base)
    _, body = c.req("POST", "/shopping-carts", {"customer_id": 7000})
    cid = body["shopping_cart_id"]
    errors, done = [], threading.Event()
    reader_obs = [[] for _ in range(readers)]

    def writer(w):
        wc = Client(base)
        for _ in range(adds):
            st, _ = wc.req("POST", f"/shopping-carts/{cid}/items", {"product_id": 1 + (w % 3), "quantity": 1})
            if st != 204:
                errors.append(st)

    def reader(r):
        rc = Client(base)
        while not done.is_set():
            st, b = rc.req("GET", f"/shopping-carts/{cid}")
            if st == 200:
                reader_obs[r].append(b["version"])

    rts = [threading.Thread(target=reader, args=(r,)) for r in range(readers)]
    wts = [threading.Thread(target=writer, args=(w,)) for w in range(writers)]
    t0 = time.perf_counter()
    for t in rts + wts:
        t.start()
    for t in wts:
        t.join()
    write_s = time.perf_counter() - t0
    done.set()
    for t in rts:
        t.join()
    _, strong = c.req("GET", f"/shopping-carts/{cid}?consistent=true")
    _, eventual = c.req("GET", f"/shopping-carts/{cid}")
    regressions = sum(1 for obs in reader_obs for a, b in zip(obs, obs[1:]) if b < a)
    return {
        "cart_id": cid, "writers": writers, "adds_per_writer": adds, "readers": readers,
        "write_errors": len(errors), "write_phase_s": round(write_s, 2),
        "expected_total_quantity": writers * adds,
        "strong_read_total_quantity": strong["total_quantity"], "strong_read_version": strong["version"],
        "eventual_read_total_quantity_after": eventual["total_quantity"],
        "lost_updates": writers * adds - strong["total_quantity"],
        "reader_observations": sum(len(o) for o in reader_obs),
        "version_regressions_seen_by_readers": regressions,
    }


def probe(base, mode, trials):
    c = Client(base)
    st, body = c.req("POST", "/debug/consistency-probe", {"trials": trials, "mode": mode})
    if st != 200:
        raise RuntimeError(f"probe {mode}: {st} {body}")
    return body


def summarize(trials):
    stale = [t for t in trials if t["stale_reads"] > 0]
    conv = sorted(t["converge_ms"] for t in stale if t["converge_ms"] >= 0)
    return {"trials": len(trials), "stale_first_reads": len(stale),
            "stale_pct": round(100 * len(stale) / len(trials), 2),
            "max_converge_ms_when_stale": conv[-1] if conv else 0}


def main():
    base, out = sys.argv[1], sys.argv[2]
    n = int(sys.argv[3]) if len(sys.argv) > 3 else 100
    m = int(sys.argv[4]) if len(sys.argv) > 4 else 500
    res = {}
    print("1) create -> read ...", flush=True)
    res["create_then_read"] = scenario_create_read(base, n)
    print("  ", summarize(res["create_then_read"]))
    print("2) add -> read ...", flush=True)
    res["add_then_read"] = scenario_add_read(base, n)
    print("  ", summarize(res["add_then_read"]["trials"]))
    print("3) concurrent writers on one cart ...", flush=True)
    res["concurrent_same_cart"] = scenario_concurrent(base)
    print("  ", {k: v for k, v in res["concurrent_same_cart"].items()})
    for mode in (("create", "update") if m > 0 else ()):  # m=0: skip (probe is DynamoDB-only)
        print(f"4) in-region probe mode={mode} x{m} ...", flush=True)
        p = probe(base, mode, m)
        res[f"probe_{mode}"] = p
        print("  ", summarize(p["results"]))
    res["summary"] = {
        "client_create_then_read": summarize(res["create_then_read"]),
        "client_add_then_read": summarize(res["add_then_read"]["trials"]),
    }
    for mode in ("create", "update"):
        if f"probe_{mode}" in res:
            res["summary"][f"probe_{mode}"] = summarize(res[f"probe_{mode}"]["results"])
    with open(out, "w") as f:
        json.dump(res, f, indent=2)
    print("->", out)


if __name__ == "__main__":
    main()

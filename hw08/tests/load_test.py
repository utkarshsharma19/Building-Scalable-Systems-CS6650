#!/usr/bin/env python3
"""HW8 required test: exactly 150 operations - 50 create_cart, 50 add_items, 50 get_cart.

Same protocol as STEP I (MySQL) so the two result files are directly comparable:
  phase 1: POST /shopping-carts                 x50  (customer_id 1..50)
  phase 2: POST /shopping-carts/{id}/items      x50  (one item into each cart created above)
  phase 3: GET  /shopping-carts/{id}            x50  (each cart, default/eventually consistent read)
Sequential, single client, one keep-alive HTTP connection; response_time = client-observed ms.

usage: load_test.py BASE_URL OUTPUT.json
"""
import http.client
import json
import random
import sys
import time
from datetime import datetime, timezone
from urllib.parse import urlparse


def main():
    base, out_path = sys.argv[1], sys.argv[2]
    u = urlparse(base)
    conn_cls = http.client.HTTPSConnection if u.scheme == "https" else http.client.HTTPConnection
    conn = conn_cls(u.hostname, u.port, timeout=10)
    random.seed(8)
    results, cart_ids = [], []

    def call(op, method, path, body=None, expect=None):
        nonlocal conn
        payload = json.dumps(body) if body is not None else None
        headers = {"Content-Type": "application/json"} if payload else {}
        ts = datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")
        t0 = time.perf_counter()
        try:
            conn.request(method, path, body=payload, headers=headers)
            resp = conn.getresponse()
            data = resp.read()
            status = resp.status
        except Exception as e:  # network error: record as a failed operation, reconnect
            print(f"{op} error: {e}", file=sys.stderr)
            conn.close()
            conn = conn_cls(u.hostname, u.port, timeout=10)
            data, status = b"", 0
        rt = (time.perf_counter() - t0) * 1000
        results.append({
            "operation": op,
            "response_time": round(rt, 2),
            "success": status == expect,
            "status_code": status,
            "timestamp": ts,
        })
        return status, data

    start = time.time()
    for i in range(50):
        status, data = call("create_cart", "POST", "/shopping-carts", {"customer_id": i + 1}, expect=201)
        cart_ids.append(json.loads(data)["shopping_cart_id"] if status == 201 else None)
    for cid in cart_ids:
        call("add_items", "POST", f"/shopping-carts/{cid}/items",
             {"product_id": random.randint(1, 1000), "quantity": random.randint(1, 5)}, expect=204)
    for cid in cart_ids:
        call("get_cart", "GET", f"/shopping-carts/{cid}", expect=200)
    elapsed = time.time() - start

    with open(out_path, "w") as f:
        json.dump(results, f, indent=2)
    ok = sum(r["success"] for r in results)
    print(f"{len(results)} operations in {elapsed:.1f}s, {ok} successful -> {out_path}")
    for op in ("create_cart", "add_items", "get_cart"):
        rts = sorted(r["response_time"] for r in results if r["operation"] == op)
        print(f"  {op:12s} n={len(rts)} avg={sum(rts)/len(rts):6.1f}ms p50={rts[len(rts)//2]:6.1f}ms max={rts[-1]:6.1f}ms")


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Part 0 check: both result files have exactly 150 operations (50 per type), and combined_results.json has both."""
import collections, json, sys
ok = True
for f in ("results/mysql_test_results.json", "results/dynamodb_test_results.json"):
    d = json.load(open(f))
    c = collections.Counter(r["operation"] for r in d)
    good = len(d) == 150 and all(c[o] == 50 for o in ("create_cart", "add_items", "get_cart"))
    ok &= good
    print(f"{f}: total={len(d)} create_cart={c['create_cart']} add_items={c['add_items']} get_cart={c['get_cart']} "
          f"success={sum(r['success'] for r in d)} failed={sum(not r['success'] for r in d)}  {'OK' if good else 'FAIL'}")
cb = json.load(open("results/combined_results.json"))
c = collections.Counter((r["database"], r["operation"]) for r in cb)
print(f"results/combined_results.json: records={len(cb)}")
for db in ("mysql", "dynamodb"):
    print(f"  {db:9s} create_cart={c[(db,'create_cart')]} add_items={c[(db,'add_items')]} get_cart={c[(db,'get_cart')]} total={sum(v for (d,_),v in c.items() if d==db)}")
ok &= len(cb) == 300
print("VERIFIED: both datasets have 150 operations, 50 of each type" if ok else "CHECK FAILED")
sys.exit(0 if ok else 1)

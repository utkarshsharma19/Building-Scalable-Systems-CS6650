import json
import sys
import time
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

ENDPOINTS = json.loads((Path(__file__).parent.parent / "endpoints.json").read_text())


def get(base, path, **params):
    url = f"{base}{path}?{urllib.parse.urlencode(params)}"
    with urllib.request.urlopen(url, timeout=600) as resp:
        return json.load(resp)


def run_job(input_url, n=3, mappers=None, verbose=False):
    mappers = mappers or ENDPOINTS["mappers"]
    t0 = time.perf_counter()

    split = get(ENDPOINTS["splitter"], "/split", url=input_url, n=n)
    t1 = time.perf_counter()

    with ThreadPoolExecutor(max_workers=n) as pool:
        mapped = list(pool.map(
            lambda i: get(mappers[i % len(mappers)], "/map", url=split["chunks"][i]),
            range(n)))
    t2 = time.perf_counter()

    reduced = get(ENDPOINTS["reducer"], "/reduce", urls=",".join(m["output"] for m in mapped))
    t3 = time.perf_counter()

    if verbose:
        print("split  ->", json.dumps(split, indent=2))
        for i, m in enumerate(mapped):
            print(f"map[{i}] ->", json.dumps(m))
        print("reduce ->", json.dumps(reduced, indent=2))

    return {
        "n": n,
        "split_s": t1 - t0,
        "map_s": t2 - t1,
        "reduce_s": t3 - t2,
        "total_s": t3 - t0,
        "split": split,
        "mapped": mapped,
        "reduced": reduced,
    }


if __name__ == "__main__":
    r = run_job(sys.argv[1], int(sys.argv[2]) if len(sys.argv) > 2 else 3, verbose=True)
    print(f"\nsplit {r['split_s']:.2f}s | map {r['map_s']:.2f}s | reduce {r['reduce_s']:.2f}s | total {r['total_s']:.2f}s")

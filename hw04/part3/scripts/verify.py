import json
import subprocess
import sys
from pathlib import Path

final_url, local_file = sys.argv[1], sys.argv[2]
root = Path(__file__).parent.parent

subprocess.run(["aws", "s3", "cp", final_url, "results/final_counts.json"], cwd=root, check=True)
subprocess.run(["go", "run", "./cmd/localcount", "-in", local_file, "-out", "results/local_counts.json"],
               cwd=root, check=True)

dist = json.loads((root / "results/final_counts.json").read_text())
local = json.loads((root / "results/local_counts.json").read_text())

print(f"\n{'':12}{'MapReduce':>12}{'Local':>12}")
print(f"{'total':12}{dist['total_words']:>12,}{local['total_words']:>12,}")
print(f"{'unique':12}{dist['unique_words']:>12,}{local['unique_words']:>12,}")

diff = {w for w in dist["counts"].keys() | local["counts"].keys()
        if dist["counts"].get(w) != local["counts"].get(w)}
print(f"mismatched words: {len(diff)}")
print("MATCH" if not diff else f"MISMATCH e.g. {sorted(diff)[:10]}")

print("\ntop 10:")
for w, c in sorted(dist["counts"].items(), key=lambda kv: -kv[1])[:10]:
    print(f"  {w:10} {c:>8,}")

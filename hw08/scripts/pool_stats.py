"""MySQL pool stats per task from the 10 s 'pool' log lines. usage: pool_stats.py START END"""
import json, subprocess, sys, time, calendar
def ts(x): return calendar.timegm(time.strptime(x, "%Y-%m-%dT%H:%M:%SZ"))
Q = ('fields @logStream, in_use, open_conns, wait_count, wait_ms | filter msg = "pool" | stats max(in_use) as max_in_use, '
     'avg(in_use) as avg_in_use, max(open_conns) as max_open, min(wait_count) as wc_start, max(wait_count) as wc_end, '
     'min(wait_ms) as wms_start, max(wait_ms) as wms_end, count(*) as samples by @logStream')
P = ["--profile", "hw8"]
qid = subprocess.check_output(["aws", "logs", "start-query", *P, "--log-group-name", "/ecs/hw8-cart-service", "--start-time", str(ts(sys.argv[1])),
                               "--end-time", str(ts(sys.argv[2])), "--query-string", Q, "--query", "queryId", "--output", "text"], text=True).strip()
while True:
    d = json.loads(subprocess.check_output(["aws", "logs", "get-query-results", *P, "--query-id", qid]))
    if d["status"] == "Complete": break
    time.sleep(2)
rows = [{c["field"]: c["value"] for c in r if c["field"] != "@logStream"} for r in d["results"]]
json.dump(rows, open("results/mysql-pool-stats.json", "w"), indent=2)
for r in rows: print(r)

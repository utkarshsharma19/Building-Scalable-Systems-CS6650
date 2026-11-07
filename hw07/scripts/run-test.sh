#!/usr/bin/env bash
# Run one headless Locust test and save CSVs under results/<name>/.
#
#   scripts/run-test.sh <name> <SyncUser|AsyncUser> <users> <spawn_rate> <duration>
#
#   scripts/run-test.sh p1-sync-normal SyncUser  5  1  30s
#   scripts/run-test.sh p1-sync-flash  SyncUser  20 10 60s
#   scripts/run-test.sh p3-async-flash AsyncUser 20 10 60s
set -euo pipefail
cd "$(dirname "$0")/.."

name=$1 class=$2 users=$3 rate=$4 dur=$5
host=${HOST:-$(terraform -chdir=terraform output -raw alb_url)}

if [[ $class == AsyncUser ]]; then
  subs=$(aws sns list-subscriptions-by-topic \
    --topic-arn "$(terraform -chdir=terraform output -raw topic_arn)" \
    --query "Subscriptions[?Protocol=='lambda'] | length(@)" --output text)
  if [[ $subs != 0 ]]; then
    echo "REFUSING: a Lambda is subscribed to the SNS topic. Set lambda_subscribed=false first." >&2
    exit 1
  fi
fi

out=results/$name
mkdir -p "$out"
echo "$(date -u +%FT%TZ) $name class=$class users=$users rate=$rate dur=$dur host=$host" | tee "$out/meta.txt"

.venv/bin/locust -f locust/locustfile.py "$class" --headless \
  -u "$users" -r "$rate" -t "$dur" -H "$host" \
  --csv "$out/locust" --only-summary 2>&1 | tee "$out/locust.log"

echo "$(date -u +%FT%TZ) finished" >> "$out/meta.txt"
python3 - "$out/locust_stats.csv" <<'EOF'
import csv, sys
for row in csv.DictReader(open(sys.argv[1])):
    if row["Name"] == "Aggregated":
        n, f = int(row["Request Count"]), int(row["Failure Count"])
        print(f"\nSUMMARY requests={n} failures={f} success={100*(n-f)/max(n,1):.1f}% "
              f"rps={float(row['Requests/s']):.1f} p50={row['50%']}ms p95={row['95%']}ms max={float(row['Max Response Time']):.0f}ms")
EOF

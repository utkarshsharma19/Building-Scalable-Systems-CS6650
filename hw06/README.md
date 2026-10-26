# HW6 – Scalability: bounded product search, breaking point, ALB + autoscaling

Builds on [HW5](../hw05/README.md): same Gin service, Terraform modules, Locust setup and Prometheus/Grafana
instrumentation, plus a new `GET /products/search?q=...` endpoint
([`product-search.yaml`](product-search.yaml), line 21).

## Where things are

| Path | What it is |
|---|---|
| [`src/product-service.go`](src/product-service.go) | Server: `sync.Map` store with 100,000 products, bounded search, HW5 endpoints, metrics, JSON logs |
| [`src/product-service_test.go`](src/product-service_test.go) | Tests that each search checks exactly 100 products, matches case-insensitively, and caps results at 20 |
| [`terraform/`](terraform/) | HW5 modules + `alb`, `autoscaling`, `dashboard` (CloudWatch). `horizontal_scaling` switches Part II ↔ Part III |
| [`locust/locustfile.py`](locust/locustfile.py) | FastHttpUser searching common terms with 0–10 ms think time |
| [`locust/shapes.py`](locust/shapes.py) | Load shapes for the exploration: `ramp`, `spike`, `sustained`, `waves` |
| [`observability/`](observability/) | Grafana + Prometheus. `docker-compose.yml` = all local; `docker-compose.aws.yml` = scrape the ECS tasks |
| [`observability/ecs-sd/`](observability/ecs-sd/) | Sidecar that writes each running task's public IP to a Prometheus `file_sd` file |
| [`scripts/`](scripts/) | `get-ip.sh`, `scale-watch.sh` (scaling timeline CSV), `stop-tasks.sh N`, `break-health.sh on/off` |

---

## 1. The search endpoint

`GET /products/search?q={query}` → `200 {"products": [...], "total_found": n, "search_time": "41.2µs"}`,
`400` if `q` is missing or not 1–100 characters, `500` via the HW5 `X-Simulate-Error: true` header.

- **Data:** 100,000 products generated at startup. Name = `"Product <Brand> <ID>"` (e.g. `Product Alpha 1`),
  brand from 10 sample values, category ID rotated with modulo over 10 names (`Electronics`, `Books`, `Home`, …).
  Brand changes every 10 IDs, so it isn't locked to the category.
- **`name` field:** the spec's `Product` schema has no name, but search needs one, so the response carries an
  extra `name` property (OpenAPI allows additional properties). It's optional on `POST /details`; if omitted, the
  existing name is kept.
- **Bounded search:** pick a random start ID, `Load` the next 100 IDs from the `sync.Map` (wrapping around), and
  test `strings.Contains(strings.ToLower(name|category), lower(q))` on each. It **always checks 100 products**, even
  after 20 matches, because the bound is on work, not on results. At most 20 products are returned.
- **`total_found`** = number of products returned (≤ 20). The spec gives it `maximum: 20`, so it counts returned
  matches, not all matches among the 100 checked.
- **Verification:** each response has an `X-Products-Checked: 100` header; Locust marks a request failed if the
  header isn't `100`. The metric `search_products_checked` feeds the "avg products checked / search" panel, and
  `go test ./...` asserts it.
- **Storage:** `sync.Map` (as the assignment asks) instead of HW5's `map` + `RWMutex`. The workload is read-only
  plus rare updates of existing keys, which is the case `sync.Map` is built for.
- **No optimization on purpose:** no pre-lowercased fields or index. The point of the experiment is a fixed cost
  per request (~16 µs of search on an M-series core, `go test -bench Search`), plus JSON encoding and the log line.

New metrics: `http_requests_in_flight` (gauge, a candidate autoscaling signal) and `search_products_checked`.

## 2. Local preview (optional, free)

```bash
cd observability
ARCH=arm64 docker compose up --build -d --scale worker=1
docker update --cpus 0.25 --memory 512m --memory-swap 512m observability-app-1   # mimic Fargate 256/512
# Locust http://localhost:8089 · Grafana http://localhost:3000 ("Product Search - RED + per task")
docker compose down
```

What I saw locally (0.25 CPU, 1 worker):

| Users | Locust RPS | Client p50 / p95 | Server p95 | Process CPU (% of 0.25) | RSS |
|---|---|---|---|---|---|
| 5 | ~370 | 1 / 53 ms | 0.47 ms | 99% | 67 MB |
| 20 | ~600 | 3 / 91 ms | 0.31 ms | 100% | 66 MB |

Locally there's no network round trip, so even 5 users saturate 0.25 vCPU. On AWS, the ~20 ms RTT per request
holds 5 users back. Note the **server p95 under 0.5 ms while the client p95 reaches 91 ms**: CPU throttling
queues requests *before* the handler's timer starts, so the app's own duration metric misses it.

---

## 3. Part II – single task, find the breaking point

```bash
# Learner Lab → AWS Details → paste credentials into ~/.aws/credentials, then:
aws sts get-caller-identity
export DOCKER_HOST=unix://$HOME/.docker/run/docker.sock

cd terraform && terraform init && terraform apply -auto-approve        # 1 task, 256 CPU / 512 MB
cd .. && IP=$(./scripts/get-ip.sh) && echo $IP

# Verify with a single search: 200, X-Products-Checked: 100, search_time in µs, total time < 20 ms server-side
curl -s -i "http://$IP:8080/products/search?q=Electro" | head -5
curl -s -o /dev/null -w "total %{time_total}s\n" "http://$IP:8080/products/search?q=Electro"

# App-level dashboard scraping the real task (separate terminal)
cd observability && docker compose -f docker-compose.aws.yml up --build
#   Grafana: http://localhost:3000   ecs-sd logs show the discovered task IP

# CloudWatch dashboard (CPU/Memory, per-task CPU, task count):
cd terraform && terraform output -raw cloudwatch_dashboard_url
```

Load tests (`locust/`, separate terminal). Each one is headless and stops on its own:

```bash
cd locust
# Test 1 – baseline: 5 users, 2 minutes
LOCUST_HEADLESS=true LOCUST_USERS=5 LOCUST_SPAWN_RATE=5 LOCUST_RUN_TIME=2m LOCUST_EXPECT_WORKERS=2 \
  TARGET_HOST=http://$IP:8080 docker compose up --build --scale worker=2
# Wait ~3 minutes so CloudWatch's 1-minute points separate the two runs.
# Test 2 – breaking point: 20 users, 3 minutes
LOCUST_HEADLESS=true LOCUST_USERS=20 LOCUST_SPAWN_RATE=20 LOCUST_RUN_TIME=3m LOCUST_EXPECT_WORKERS=2 \
  TARGET_HOST=http://$IP:8080 docker compose up --build --scale worker=2
```

(Or drop the `LOCUST_*` variables and use the UI at <http://localhost:8089> to get Locust's charts for screenshots.)

**Capture, for each test, over the same time window:**
1. Locust final stats (RPS, p50/p95/p99, failures) and Locust charts.
2. Grafana: Rate, Errors, Duration, process CPU, in-flight.
3. CloudWatch: CPU and Memory utilization. Set the period to 1 minute and the range to cover both tests.

**Vertical-scaling check (mock interview "256 → 512?"):**
`terraform apply -var task_cpu=512 -var task_memory=1024`, re-run Test 2, and set the Grafana `Task vCPU` dropdown to 0.5.

**Creative stress ideas for Part II:** 40 / 80 users; one long 10-minute run (does memory stay flat?); a
`spike` shape against the single task (`LOCUSTFILE=locustfile.py,shapes.py SHAPE=spike`); Locust with 1 worker
vs 4 (rule out the load generator as the bottleneck).

## 4. Part III – ALB + autoscaling

Read the whole section first. **Run everything below in one go.**

```bash
cd terraform && terraform apply -auto-approve -var horizontal_scaling=true    # ALB, target group, 2–4 tasks, 70% CPU target
ALB=$(terraform output -raw alb_url) && echo $ALB
curl -s -i "$ALB/products/search?q=Books" | head -3                            # wait until both targets are healthy

# Terminal A: scaling timeline (desired/running/healthy/unhealthy every 15s → results/*.csv)
./scripts/scale-watch.sh
# Terminal B: Grafana scraping every task (per-task rate/CPU = load distribution)
cd observability && docker compose -f docker-compose.aws.yml up --build
# Terminal C: the Part II breaking test, through the ALB. It needs to run longer than 3 min for a
# scale-out to show up (alarm ~3 min + task start ~1 min), so use the 20-minute sustained shape:
cd locust && LOCUSTFILE=locustfile.py,shapes.py SHAPE=sustained TARGET_HOST=$ALB docker compose up --build --scale worker=2
```

**Core test, what to record** (screenshots + the `scale-watch` CSV):
- ECS service → Tasks: desired count 2 → 3 → 4, and when each step happened.
- Target group → Targets: new targets go `initial` → `healthy` (2 checks × 30 s).
- CloudWatch: CPU per task (the Container Insights widget) before and after each scale-out, ALB request count,
  target response time, healthy hosts.
- Locust: p50/p95 and failures compared with Part II's Test 2. Do any requests fail while scaling?
- Grafana: per-task request rate (does the ALB spread load evenly?), p95, in-flight. **Note when duration and
  in-flight started rising compared with when the CPU alarm fired** (for the CPU-vs-request-metric question).

**Resilience:** during the run, `./scripts/stop-tasks.sh 1` (or ECS console → Tasks → Stop). Watch the target go
`draining`, the ECS service start a replacement, and Locust's failure count.

**Exploration** (each `terraform apply` resets the service to `min_capacity`, so every experiment starts the same way):

| Experiment | Command |
|---|---|
| Target 50% / 90% | `terraform apply -var horizontal_scaling=true -var target_cpu=50` (then `=90`) |
| Min/max | `-var min_capacity=1 -var max_capacity=6` |
| Cooldowns | `-var scale_out_cooldown=60 -var scale_in_cooldown=60` |
| Gradual ramp 5→40 | `SHAPE=ramp` |
| Sudden spike 5→40→5 | `SHAPE=spike` |
| Waves faster than cooldown | `SHAPE=waves` |
| More users | UI or `LOCUST_USERS=60` |
| Stop several tasks | `./scripts/stop-tasks.sh 2` (or `3`) |
| All targets fail health checks | `./scripts/break-health.sh on` … observe … `./scripts/break-health.sh off && terraform apply -var horizontal_scaling=true` |

Things worth predicting before you run them: what the ALB does when **no** target is healthy (it fails open and
keeps routing to all of them), and whether ECS replaces tasks that fail ELB health checks.

**Tear down when done:** `cd terraform && terraform destroy -auto-approve -var horizontal_scaling=true`.

---

## 5. Results

The write-up with results and figures is [`HW6-Utkarsh-Sharma.docx`](HW6-Utkarsh-Sharma.docx). All screenshots are in
[`screenshots/`](screenshots/) (`p2-*` Part II, `p3-*` Part III core test, `x1/x2/x3-*` exploration), and the scaling
timeline logged by `scale-watch.sh` is in [`results/`](results/).

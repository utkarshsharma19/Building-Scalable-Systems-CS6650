# HW7: Sync vs. async order processing (SNS/SQS/ECS) and Lambda

**Report:** [REPORT.md](REPORT.md) · PDF: [HW7-Utkarsh-Sharma.pdf](HW7-Utkarsh-Sharma.pdf) · all numbers: [results/FINDINGS.md](results/FINDINGS.md)

```
Sync:   Customer → ALB → Receiver → Payment (3 s, one at a time) → 200
Async:  Customer → ALB → Receiver → SNS → 202 (< 100 ms)
                                      ├→ SQS → Processor (N worker goroutines) → Payment (3 s)
                                      └→ Lambda (Part III only, curl tests only)
```

| Path | What it is |
|---|---|
| `src/cmd/receiver` | `POST /orders/sync`, `POST /orders/async`, `GET /health`, `GET /stats` |
| `src/cmd/processor` | SQS long-poll loop; `WORKERS` goroutines verify payments; `/health`, `/stats` |
| `src/cmd/lambda` | SNS-triggered Lambda, same 3 s payment |
| `src/internal/order` | `Order`/`Item` types, simulated payment, env helpers |
| `terraform/` | VPC (10.0.0.0/16, public 10.0.1-2.0/24, private 10.0.10-11.0/24, 1 NAT), ALB, ECS (256 CPU/512 MB), SNS, SQS, Lambda |
| `locust/locustfile.py` | `SyncUser`, `AsyncUser` (wait 100–500 ms) |
| `scripts/` | `run-test.sh`, `queue-watch.sh`, `build-lambda.sh`, `lambda-orders.sh` |

**How the bottleneck is simulated.** A sleeping goroutine doesn't hold a thread, so `time.Sleep` alone would let every sync request "pay" in parallel. The receiver uses a buffered channel of size 1 as the payment processor: a request must put a token in before sleeping 3 s, so only one payment runs at a time (0.33 orders/s). The processor uses the same idea with a channel of size `WORKERS`, and it only asks SQS for as many messages as it has free workers. Otherwise messages would sit in memory past the 30 s visibility timeout and be delivered twice.

## Setup (once per lab session)

1. Start the Learner Lab, copy the credentials into `~/.aws/credentials`, and check them: `aws sts get-caller-identity`.
2. Start Docker Desktop.
3. One-time: `python3 -m venv .venv && .venv/bin/pip install locust`, then `scripts/build-lambda.sh`.
4. Deploy: `cd terraform && terraform apply` (about 5 min), then smoke-test it:

```bash
ALB=$(terraform -chdir=terraform output -raw alb_url)
curl -s $ALB/health
curl -s -X POST $ALB/orders/async -H 'Content-Type: application/json' \
  -d '{"customer_id":123,"items":[{"product_id":"p001","quantity":1,"price":19.99}]}'
```

**Always finish with `terraform destroy`.** The NAT gateway and the ALB bill by the hour.

## Part II experiments

Every result goes in `results/<name>/` (Locust CSVs, `meta.txt`, `queue.csv`). Take the CloudWatch screenshots into `screenshots/`.

| # | Phase | Command | Record |
|---|---|---|---|
| 1 | 1: sync normal | `scripts/run-test.sh p1-sync-normal SyncUser 5 1 30s` | success %, p50/p95, req/s |
| 2 | 1: sync flash | `scripts/run-test.sh p1-sync-flash SyncUser 20 10 60s` | same; note timeouts and 504s |
| 3 | 3: async flash, 1 worker | in a second terminal `scripts/queue-watch.sh results/p3-async-w1/queue.csv`, then `scripts/run-test.sh p3-async-w1 AsyncUser 20 10 60s` | acceptance %, req/s, queue growth/s |
| 4 | 5: 5 / 20 / 100 workers | for N in 5 20 100: `aws sqs purge-queue --queue-url $(terraform -chdir=terraform output -raw queue_url)`, `terraform -chdir=terraform apply -var processor_workers=N`, wait until the new task is RUNNING, then the queue watcher + `scripts/run-test.sh p5-w$N AsyncUser 20 10 60s` | peak depth, time to drain, processing rate, CPU/memory |

Notes:
- With 1 worker, a 60 s flash sale leaves about 3,000 messages that take about 2.5 h to drain. Stop `queue-watch.sh` after about 5 minutes, report the growth rate, then purge the queue.
- Processing rate = slope of `visible` while draining, or `completed` from the processor's `stats` log lines (CloudWatch Logs `/ecs/hw7-processor`).
- Resource utilization: CloudWatch → Container Insights → ECS service `hw7-processor` (CPU %, memory %).
- CloudWatch screenshot: `terraform -chdir=terraform output -raw sqs_metrics_console` opens `ApproximateNumberOfMessagesVisible` (set the period to 1 min, statistic Maximum). It lags a few minutes behind `queue-watch.sh`.

## Part III: Lambda (curl only, never Locust)

```bash
scripts/build-lambda.sh
terraform -chdir=terraform apply -var deploy_lambda=true -var lambda_subscribed=true
scripts/lambda-orders.sh send 5      # first one is a cold start
scripts/lambda-orders.sh burst 3     # parallel invocations, each may cold-start
# wait 10+ minutes idle, then:
scripts/lambda-orders.sh send 2      # cold again?
scripts/lambda-orders.sh report      # REPORT lines, COLD = has Init Duration
terraform -chdir=terraform apply -var deploy_lambda=true -var lambda_subscribed=false   # before any Locust run
```

`run-test.sh` refuses to run `AsyncUser` while a Lambda is subscribed to the topic.

While the Lambda is subscribed, SNS delivers each order to both SQS and Lambda, so the ECS processor still handles it too. That's fine for 5–10 curl orders.

# HW7 findings: Utkarsh Sharma (us-west-2, Learner Lab, 2026-11-07)

Locust ran from a laptop (Boston) to the ALB in us-west-2, so latencies include about 80 ms of round trip.
Raw data: `results/<run>/` (Locust CSVs, `queue.csv` sampled every 5 s, processor stats logs). Charts: `charts/`. CloudWatch graphs: `screenshots/`.

## Phase 1: synchronous

| Run | Users / spawn | Duration | Orders completed | Success* | Throughput | p50 | p95 | Max |
|---|---|---|---|---|---|---|---|---|
| p1-sync-normal | 5 / 1 per s | 30 s | 9 | 100% | 0.3/s | 12 s | 15 s | 14.7 s |
| p1-sync-flash | 20 / 10 per s | 60 s | 19 | 100% | 0.3/s | 30 s | 56 s | 56.2 s |

\*Locust counts only finished requests. When a test stops, requests still waiting in the payment line are dropped, not counted as failures. At the end of the flash test the receiver had **19 customers still waiting** (`sync_waiting=19` in its log).

- Even the "normal" load is too much for sync: 5 users want about 1–2 orders/s, and the payment processor completes 0.33/s, so every customer queues behind the others (p50 of 12 s instead of 3 s).
- Flash sale: each customer waits about (customers ahead) × 3 s. With 20 in line that's about 60 s, exactly the ALB idle timeout. A longer test would start returning 504s. CloudWatch ALB TargetResponseTime peaks at 56 s (`screenshots/p1-sync-alb-latency.png`).
- The receiver kept working on the 19 queued orders after Locust gave up: the server can't tell that the customer has left.

## Phase 2: the math

- Payment processor: 1 order per 3 s, so **maximum throughput = 0.33 orders/s, whatever the number of customers** (measured: 0.3/s in both runs).
- Demand 20 orders/s, so **about 19.7 orders/s are lost** (98%+ of demand).
- What we can change is not the payment speed but *when we answer the customer*: accept now, pay later (async).

## Phase 3/4: async, 1 worker

| Run | Orders accepted | Acceptance | Throughput | p50 | p95 | Peak queue |
|---|---|---|---|---|---|---|
| p3-async-w1 | 2,883 | 100% | 48.8/s | 100 ms | 120 ms | 2,897 |

- Async accepted **48.8 / 0.3 ≈ 160×** more orders per second than sync, with replies about 300× faster (0.1 s vs 30 s median).
- Processor measured at 0.3–0.4 orders/s (processor stats log), as expected for 1 worker.
- Queue growth: 48.8 − 0.33 ≈ **48.5 messages/s** measured (the assignment's 60/s load gives 59.7/s).
- Time to clear our 60 s backlog: 2,897 / 0.33 ≈ 8,700 s ≈ **145 min**. At the assignment's full hour of 60/s: about 215k messages / 0.33 ≈ 645k s ≈ **10,700 min (7.5 days)**, longer than the 4-day message retention, so orders would be **deleted unprocessed**.
- We purged the queue at 21:39 instead of waiting 2.4 h (the cliff in `screenshots/p3-async-w1-sqs-depth.png`).

## Phase 5: worker scaling

Same flash load each time (AsyncUser, 20 users, 60 s, about 49 orders/s arriving, 100% accepted). One processor task with 256 CPU / 512 MB; only `WORKERS` changes.

| Workers | Theory (N/3 s) | Measured rate | Peak queue | Queue empty after test end | Max CPU | Max memory |
|---|---|---|---|---|---|---|
| 1 | 0.33/s | 0.3–0.4/s | 2,897 | ~145 min (computed; purged) | 2.9% | 1.4% |
| 5 | 1.67/s | W5_RATE | W5_PEAK | W5_DRAIN | W5_CPU | W5_MEM |
| 20 | 6.67/s | 6.0–8.0/s (median 6.6) | 2,441 | 412 s (6.9 min) | 8.0% | 1.6% |
| 100 | 33.3/s | 30–34.6/s (median 32.6) | 837 | 66 s | 29.3% | 1.6% |

- The rate grows **linearly with workers** (N / 3 s), because each worker spends its 3 s sleeping, not computing.
- Even 100 workers (33/s) can't keep up with 49/s arriving, so the queue still builds (+16/s, about 900 by the end of the sale), but it drains about a minute later.
- **Minimum workers to prevent buildup at 60 orders/s: 60 × 3 = 180** (Little's Law: in-flight = arrival rate × time per order). For our measured 49/s: 147.
- **Resources aren't the limit.** CPU scales with workers (2.9% → 8% → 29% for 1 → 20 → 100) because of SQS receive/delete calls, and memory stays flat at about 1.5%. 180 workers would use roughly 50% CPU, so one 0.25 vCPU task can handle it. The real limit is the payment provider's 3 s, not our container.
- Measured rates oscillate between 6 and 8 per 10 s window because orders finish in batches of up to 10 (one ReceiveMessage call).

## Part III: Lambda (SNS → Lambda, no SQS)

Deployed `hw7-order-processor`: Go on `provided.al2`, 512 MB, triggered directly by `order-processing-events`. Ten orders were sent with curl through the same `/orders/async` endpoint (`scripts/lambda-orders.sh`); there was no Locust. Raw REPORT lines: `results/lambda-report.txt`; graph: `screenshots/p3-lambda-lambda.png`.

| Time (local) | Request | Start | Duration | Billed | Init Duration |
|---|---|---|---|---|---|
| 17:06:40 | order 1 | **cold** | 3,004 ms | 3,077 ms | 72.0 ms |
| 17:06:42 | order 2 (2 s later) | **cold** | 3,004 ms | 3,074 ms | 69.0 ms |
| 17:06:44–49 | orders 3–5 | warm | 3,002–3,004 ms | 3,002–3,004 ms | — |
| 17:06:59 | burst of 3, two of them | warm | 3,004 ms | 3,004 ms | — |
| 17:07:00 | burst of 3, the third | **cold** | 3,004 ms | 3,088 ms | 83.5 ms |
| 17:08:08–11 | 2 orders, ~1 min later | warm | 3,002–3,003 ms | 3,002–3,003 ms | — |
| IDLE_ROWS |

**How often did cold starts occur?** 3 of the first 10 invocations, and not only on "the first request":
- Order 2 arrived 2 s after order 1, while order 1 was still in its 3 s payment. Lambda runs one event per instance, so it had to **start a second instance, which was cold**. Cold starts follow *new concurrency*, not just idle time.
- In the burst of 3, two orders reused the two warm instances, and the third needed a new, cold one.
- After more than 10 minutes idle: IDLE_RESULT

**Overhead:** an Init Duration of 69–84 ms on 3,000 ms of work is **2.3–2.8%**, matching the assignment's 73 ms example. It's billed: billed duration includes the init time (3,074–3,088 ms vs. about 3,003 ms warm). Max memory used was 19 MB of 512 MB.

**Does it matter for 3 s payment processing?** No. It's background work that the customer never waits on (they already got their 202). It would matter on a request path with a latency budget of about 100 ms.

SNS → Lambda delivery (`queued_ms − payment_ms` in our function's log): about 80–115 ms warm and 290–400 ms cold.

### Cost

Prices: Lambda $0.20 per 1M requests and $0.0000166667 per GB-s; free tier 1M requests and 400k GB-s per month. Measured cost per order: about 3.0 s × 0.5 GB = **1.5 GB-s** (3.08 s on a cold start).

| Orders / month | GB-s | Lambda cost | ECS processor task (always on) |
|---|---|---|---|
| 10,000 | 15,000 | **$0** | ~$9 (1 task) |
| 100,000 | 150,000 | **$0** | ~$9 |
| 266,667 | 400,000 (free tier used up) | **$0** | ~$9 |
| 1,000,000 | 1,500,000 | ~$18.33 | ~$9 |
| One flash-sale hour (216,000 orders at 60/s) | 324,000 | $0 inside the free tier | ~$9, but the queue takes 7.5 days with 1 worker |

- **Break-even with the assignment's $17/month:** (17 / 0.0000166667 + 400,000) / 1.5 ≈ **945,000 orders/month**. The assignment's "~1.7M requests" is off: at 1.7M orders, Lambda costs about $36, because GB-s dominates and the request charge is only $0.14.
- **Lambda replaces only the processor.** The receiver still needs its ECS task and the ALB, so the real saving is about $9/month (one 0.25 vCPU / 0.5 GB Fargate task at about $0.0123/h), not $17.
- **The bigger fixed costs are the ALB (~$16/month) and the NAT gateway (~$33/month + data)** that the private-subnet design requires. Our Part II stack actually costs about $65–70/month at list price. (Fargate, ALB and NAT prices are us-west-2 list prices as of 2026 and approximate; we didn't fetch a bill.)

### Recommendation: switch the processor to Lambda, but keep a queue in front of it

- **For:** about $0 below ~267k orders/month. No worker-count tuning (Phase 5's "180 goroutines" decision disappears, because Lambda scales one instance per concurrent order). No processor health checks. 2–3% cold-start overhead doesn't matter for background payments.
- **Against:** with SNS → Lambda directly, a failed order gets 2 retries and is then **dropped**. There's no backlog to replay, no visibility timeout, and no batching. A flash sale at 60/s × 3 s needs about **180 concurrent Lambdas**. A new or Learner Lab account's concurrency limit (often 10) would throttle that, and throttled async events are retried and eventually dropped.
- **What we'd ship: SNS → SQS → Lambda (SQS event source).** It keeps SQS's durability, retention and dead-letter queue. Lambda polls and scales automatically, and `maximum_concurrency` caps it at what the payment provider allows. Alternatively, add an on-failure destination or DLQ to the direct SNS → Lambda path.

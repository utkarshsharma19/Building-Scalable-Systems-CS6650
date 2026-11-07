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

(filled in as the runs finish)

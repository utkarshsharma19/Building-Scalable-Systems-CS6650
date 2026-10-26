# 120-user follow-up: 512 CPU units, 1,024 MiB

2026-10-26, approximately 09:29:52–09:34:52 PDT; 5 minutes, spawn rate 20 users/s, two existing Locust workers, same search workload and 0–10 ms wait time. A new single ECS task was provisioned after the earlier teardown; underlying host differences are a comparison limitation.

| Metric | 512 units / 60 users | 512 units / 120 users |
|---|---:|---:|
| Whole-run throughput | 1,487.6 req/s | 2,107.7 req/s |
| Client p50 | 32 ms | 40 ms |
| Client p95 | 52 ms | 100 ms |
| Client p99 | 96 ms | 180 ms |
| Requests | 267,708 | 632,473 |
| Failures | 0 | 0 |
| CPU evidence | 72–75% steady minute averages | 95.2% average / 98.9% maximum for 09:30 PDT only |

Throughput increased 41.7% with twice the concurrency; p95 increased 92.3%. Relative to the 256-unit/60-user baseline (1,056 req/s), throughput almost doubled. The earlier 60-user result was not a hard network throughput ceiling: more concurrent requests used CPU headroom while hiding round-trip wait time. At 120 users, ~2.1k req/s and the one available high-CPU minute are consistent with approaching a CPU ceiling. This is the observed operating point, not proof of an absolute maximum. No 240-user test was run.

As a rough closed-loop check, 120 users / (50.55 ms mean response + 5 ms mean think time) gives about 2,160 req/s, close to the observed average. Under saturation, adding users mostly increases waiting rather than useful capacity. Prefer a load below the ceiling when p95 latency matters.

Evidence limitation: CloudWatch GetMetricStatistics returned the 09:30 minute before the lab credentials were revoked. Later reads failed with an explicit service-control-policy deny; ECS also returned a voc-cancel-cred denial. Thus full-run CPU cannot be claimed. Worker CPU was about 30% each in the early sample, making load-generator CPU saturation less likely.

Teardown: Terraform destroy was attempted but failed because AWS revoked access. Final removal of this follow-up deployment is UNCONFIRMED. The earlier deployment had been fully destroyed before this run.

# Vertical scaling: 512 CPU units

Run: 2026-10-26 08:56–08:59 PDT, 60 users, two Locust workers.

| Metric | 256 CPU baseline | 512 CPU measured |
|---|---:|---:|
| Throughput | 1,056 req/s | 1,487.6 req/s (whole run) |
| Client p95 | 90 ms | 52 ms |
| CPU utilization | ~99% | 71.7–74.6% (steady minute averages) |

512 run: 267,708 requests, zero failures, p50 32 ms, p99 96 ms.
Throughput improved 40.9%; p95 decreased 42.2%. The final instantaneous RPS displayed by Locust is 1,512.8; the report uses whole-run average throughput.

CPU headroom is consistent with a latency/concurrency constraint, but does not prove network saturation. A 120-user run was not performed; it would help locate the new ceiling.

Evidence: saved Locust JSON and HTML report, CloudWatch CPU JSON, and `screenshots/x4-512-*`. CloudWatch images are AWS metric-widget exports of dashboard panels for 08:54–09:01 PDT, rather than console screenshots. Locust charts retain their last RPS value after the test stopped; the extension after 08:59 is not additional traffic.

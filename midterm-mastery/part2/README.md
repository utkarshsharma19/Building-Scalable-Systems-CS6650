# Midterm Mastery Part 2: Empirical evaluation of the FTP_6650 file-transfer service

**The presentation itself is [`EXPERIMENT_PRESENTATION.md`](EXPERIMENT_PRESENTATION.md)** (talking points for a 4–5 minute talk, with the charts it refers to).

- Service fork (kept out of this repo, as required): https://github.com/utkarshsharma19/file-transfer-playservice, branch `experiments`
- What I changed in the service and why: [`SERVICE_CHANGES.md`](SERVICE_CHANGES.md)
- How the screenshots were taken: [`SCREENSHOTS.md`](SCREENSHOTS.md)

Deployed with the repo's `scripts/deploy.sh` to AWS Learner Lab, `us-west-2`: ALB → gateway (Fargate 0.25 vCPU) → NLB (UDP) → backend (Fargate 0.25 vCPU) → S3. All tests were driven from my laptop.

## Layout

| Path | What |
|---|---|
| `scripts/env.sh` | Reads the gateway URL, bucket and prefix from the live Terraform outputs |
| `scripts/make_files.sh` | Makes random test files (1 KB, 16 KB, 64 KB, 256 KB, 1 MB), records SHA-256s and uploads them to S3 |
| `scripts/exp1_loss_sweep.py`, `scripts/exp1_run.sh` | Experiment 1: sequential downloads, each checked against its SHA-256 |
| `scripts/exp2_inflight.sh`, `locust/locustfile.py` | Experiment 2: Locust load at a fixed `MAX_INFLIGHT` |
| `scripts/exp3_autoscale.sh`, `scripts/scale_watch.sh`, `scripts/cloudwatch_metrics.sh` | Experiment 3: step load, a 15 s task-count timeline, and CloudWatch metrics plus scaling activities |
| `scripts/logs_insights.sh` | Pulls my `FETCH_STATS` / `BACKEND_STATS` log lines out of CloudWatch Logs Insights |
| `scripts/run_phases_2_3.sh` | Redeploys with each setting and runs the experiment, unattended |
| `scripts/plot.py` | Builds every chart in `charts/` from `results/` |
| `results/` | Raw data: CSV, JSON, Locust HTML reports and run logs |
| `results/local-pilot/` | Local pilot run before using AWS (same binaries, `-dir` file source) |
| `charts/` | The charts used in the presentation |

## What was fixed and what was varied

| | Fixed | Varied |
|---|---|---|
| Exp 1: loss × file size | 1 request at a time; retry policy at defaults (5 OPEN attempts, 5 RETY rounds, 2 s read timeout); `WORKERS=8`; `MAX_INFLIGHT=4` | backend `SEND_LOSS` ∈ {0, 1, 5}%, `RECV_LOSS` ∈ {0, 20}%; file size 1 KB to 1 MB; 8 reps each (160 downloads) |
| Exp 2: gateway admission (`MAX_INFLIGHT`) | 16 KB file (reliable when idle); auto scaling pinned at 2 gateway + 2 backend tasks; loss 0 | `MAX_INFLIGHT` ∈ {4, 16}; 10 vs 40 Locust users |
| Exp 3: auto scaling | defaults: gateway on `ALBRequestCountPerTarget` target 100/min, backend on CPU target 60%, 2..4 tasks, cooldowns 60 s out / 120 s in; 64 KB file | time: 5 users (2 min) → 40 users (10 min) → 1 user (16 min) |

## Findings

### Exp 1: the protocol, not the loss setting, sets the size limit
Success rate on AWS (8 downloads per cell, verified by SHA-256):

| file (64-byte chunks) | 0 / 0 | send 1% | send 5% | recv 20% |
|---|---|---|---|---|
| 1 KB (16) | 100% | 100% | 100% | 100% |
| 16 KB (256) | 100% | 100% | 100% | 100% |
| 64 KB (1,024) | 62% | 100% | 100% | 75% |
| 256 KB (4,096) | 25% | 0% | 0% | 0% |
| 1 MB (16,384) | 25% | 0% | 0% | 0% |

- **Even with 0% simulated loss, files of 256 KB and up mostly fail with 504 on AWS.** The backend fires one datagram per 64 bytes with no pacing (16,384 for 1 MB), so the real network, the NLB and the 0.25 vCPU task drop a large share on their own. The simulated loss is small next to that real loss. On localhost the same 1 MB file succeeded every time at 0% and 1% loss (`results/local-pilot`).
- **Every missing chunk costs a full re-read of the file from S3.** `handleRetry` fetches the whole object to send one 64-byte chunk. For the 40 downloads in the 0% run, the backend made **3,518 S3 GETs** (88 per download) and sent 177,686 datagrams.
- **Most retries are thrown away.** One RETY datagram goes out per missing chunk, all at once. The backend's job queue holds `workers*4 = 32` messages, so most RETYs are dropped as "Overloaded". That was **44,466 dropped in the 0% run and 83,828 at 5% send loss**, with the queue at its 32-slot cap every time (`results/exp1_*_backend_stats.csv`). Five retry rounds can never recover thousands of chunks when only about 32 get through per burst.
- **Latency has a floor of about 2 s.** The client only knows the server is done when the socket has been quiet for `readTimeout` = 2 s. Each retry round adds another 2 s or more, so a failed large file takes 15–20 s before the 504.
- Receive loss mostly costs time: a dropped OPEN costs a 2 s timeout and a redial. It hurts large files because RETYs are dropped too.

Charts: `charts/exp1_loss_vs_size.png`, `charts/exp1_retry_volume.png`.

### Exp 2: `MAX_INFLIGHT` is the gateway's admission control
- AWS, `MAX_INFLIGHT=4`, 2 gateways, 40 users, 16 KB file: **1,116 requests, 530 of them 503 "gateway busy" (47.5%)** and no 504s. Only 2 × 4 = 8 downloads can run at once and each takes at least 2 s, so capacity is about 4 downloads/s. Everything else waits the 5 s `QUEUE_WAIT` and is refused. (`results/invalid-exp2-locust-stalled/exp2_inflight4_u40.html`. This run completed normally; see the note below.)
- Local A/B with the same binaries (1 gateway, 1 backend, 40 users, 60 s, `results/exp2-local/`):

| MAX_INFLIGHT | requests | 503s | median latency |
|---|---|---|---|
| 4 | 392 | 276 (70%) | 5.0 s (the 5 s queue wait) |
| 16 | 464 | **0** | 4.0 s |

  With 16 slots, every request gets admitted and throughput rises. The 503s disappear, but the queue moves into latency instead. The backend's 32-message queue was not overrun at 16 KB, but the Exp 1 numbers show it would be with bigger files. Raising `MAX_INFLIGHT` moves the bottleneck from the gateway to the backend; it doesn't remove it.
- Note: my first AWS attempt at `MAX_INFLIGHT=16` is not valid. Locust's CSV writer blocked on my iCloud-synced Desktop and froze the simulated users. The data is quarantined in `results/invalid-exp2-locust-stalled/`. `scripts/exp2_inflight.sh` now writes Locust output to a local cache folder.

### Exp 3: the two scalers watch different things, and neither watches failures
(`charts/exp3_autoscale_timeline.png`, `results/exp3_*`)
- 40 users pushed **ALBRequestCountPerTarget to about 175/min against a target of 100**. The gateway scaled 2 → 4 at minute 7.6, about 5 minutes after the load step. Target tracking needs 3 consecutive one-minute breaches, and then the tasks still have to start.
- After the scale-out, 503s fell sharply (8 → 16 inflight slots in total). Backend CPU then rose to **52%**, because twice as many downloads reached it. That is still under the 60% target, so **the backend never scaled**, although 169 requests ended in 504 and the backend dropped **141,599** RETY datagrams. Most of the backend's work is waiting on S3 and UDP, not CPU.
- The ALB metric counts **503 and 504 responses as "requests"**, so failing traffic still drives the gateway's scaling.
- Scale-in was slow and gradual: the load dropped at minute 12.2, and the first scale-in (4 → 3) came at minute 29, about 17 minutes later. CloudWatch's "AlarmLow" needs 15 minutes of low readings.

## Overall takeaways
1. The weak point is the reliability protocol, not compute: no pacing on sends, one RETY per chunk, a full S3 re-read per RETY, and a 32-slot drop-tail queue.
2. Scaling on CPU doesn't fit this backend. A custom metric such as overload drops, retry count or the gateway's 504 rate would scale it when it is actually failing.
3. `MAX_INFLIGHT` should be sized against the backend's capacity (`WORKERS` × tasks), or it only trades 503s for 504s.

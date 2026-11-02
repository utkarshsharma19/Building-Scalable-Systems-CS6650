# EXPERIMENT PRESENTATION: Midterm Mastery Part 2 (talking points, about 4.5 minutes)

*This file is the experiment presentation. Each section is one "slide": show the image, say the bullets.*

---

## 0. Setup (30 s)
- Service: HTTP gateway (ALB, Fargate) → custom UDP protocol through an NLB → backend (Fargate) → S3. Files go out as 64-byte chunks with OPEN/RETY retries.
- I added two log lines: `FETCH_STATS` per download in the gateway and `BACKEND_STATS` every 10 s in the backend. They count retries, S3 reads and queue drops, and I pull them out with CloudWatch Logs Insights.
- Three experiments, each with one or two variables changed and everything else fixed:
  1. loss × file size (reliability)
  2. gateway `MAX_INFLIGHT` under load (admission control)
  3. the two auto scalers under a step load

---

## 1. Loss vs. file size (1 min 30 s) — `charts/exp1_loss_vs_size.png`, `charts/exp1_retry_volume.png`
- Fixed: one request at a time, default retries, 8 workers. Varied: send loss 0/1/5%, receive loss 20%, sizes 1 KB to 1 MB. 8 SHA-checked downloads per cell.
- **Surprise: at 0% simulated loss, 256 KB and 1 MB still fail 75% of the time on AWS.** Locally they never fail. The backend sends 16k datagrams in a burst with no pacing, so the real network drops them.
- Why retries can't fix it (my backend counters):
  - the client sends **one RETY per missing chunk, all at once**
  - the backend queue holds **32** messages, so **44k RETYs were dropped** at 0% loss and 84k at 5%
  - each RETY that does get in **re-reads the whole file from S3**: 3,518 S3 GETs for 40 downloads
- Latency floor is **2 s** (the client's "server went quiet" timeout). Each retry round adds about 2 s, so failures take about 20 s.
- Takeaway: the size limit comes from the protocol design, not from the loss knob.

---

## 2. MAX_INFLIGHT (1 min) — README table, `results/exp2-local`, AWS `exp2_inflight4_u40.html`
- Fixed: 16 KB file, 2 gateway + 2 backend tasks (scaling pinned), no loss. Varied: `MAX_INFLIGHT` 4 vs 16, 10 vs 40 users.
- AWS, `MAX_INFLIGHT=4`, 40 users: **47.5% of requests got 503 "gateway busy"**. Only 8 downloads fit at once, each takes ≥ 2 s, and the rest time out after the 5 s queue wait.
- `MAX_INFLIGHT=16`: **0 503s** and higher throughput, but the waiting moves into latency and onto the backend's 32-slot queue.
- Takeaway: `MAX_INFLIGHT` has to be sized against backend capacity (`WORKERS` × tasks), or it just turns 503s into 504s.
- (Honesty note: my first AWS run at 16 froze because Locust was writing to an iCloud-synced folder. I threw that data out and documented it.)

---

## 3. Auto scaling (1 min 30 s) — `charts/exp3_autoscale_timeline.png`
- Defaults: gateway scales on ALB requests/target (target 100/min), backend on CPU (target 60%), 2–4 tasks. Load: 5 → 40 → 1 users over 28 min.
- Gateway: requests/target hit **about 175**, and it **scaled 2 → 4 about 5 min after the step** (3 one-minute breaches, then task start-up). 503s fell right away.
- Backend: **never scaled.** CPU peaked at 52%, under 60%, while it was dropping 141k datagrams and returning 504s. Its work is mostly I/O (S3 and UDP), and CPU can't see that.
- The ALB metric counts failed requests too, so the gateway scales on traffic, not on health.
- Scale-in: **17 min** after the load dropped, and only one task (4 → 3), because of the 15-minute low alarm.

---

## 4. Conclusions (30 s)
1. The bottleneck is the reliability protocol: no send pacing, per-chunk RETY bursts, a full S3 re-read per RETY, and a 32-slot drop-tail queue.
2. Scale the backend on what fails (overload drops, retry rate, 504s) through a custom CloudWatch metric, not on CPU.
3. Gateway concurrency and backend workers have to be tuned together.

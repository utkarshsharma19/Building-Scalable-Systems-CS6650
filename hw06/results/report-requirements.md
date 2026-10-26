# Submission coverage

The main HW6-Utkarsh-Sharma.docx is the concise submission. The older detailed report is preserved as results/HW6-detailed-evidence.docx and is supporting evidence, not the 1–2-page submission.

| Requirement | Location in brief report |
|---|---|
| Increased-load behavior | Part II results table and interpretation |
| Optimization vs scaling evidence | Fixed 100 checks, CPU saturation, flat memory, low handler p95; optimization remains possible |
| CloudWatch scaling decisions | Measured capacity, 70% target, 120-user CPU evidence with limitation |
| HW5 and CloudWatch side by side for same test | Fig. 1, matching UTC/PDT times and dashboard comparison |
| How horizontal scaling addressed Part II | Components and result paragraph |
| ALB, target group, auto scaling roles | Components and result paragraph |
| Horizontal vs vertical trade-offs | Two-sentence comparison |
| Load-pattern predictions | Spike/ramp results, scheduled capacity, untested 90% prediction |
| CPU vs request metric backed by core scale-out | Core evidence, Fig. 2, and explicit metric-choice paragraph |
| Creative stress tests and findings | 60/120-user concurrency tests, spikes, ramp/policy changes, stop 1/3 tasks, broken health checks |

Request-count target is correctly expressed as 42,000 requests per target per minute (~700/s), is a proposal rather than a tested policy, and does not promise to eliminate scaling delay.

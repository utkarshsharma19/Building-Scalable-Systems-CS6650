# Screenshots

Save every screenshot into `midterm-mastery/part2/screenshots/`. On macOS, run the command and then click the window (or drag a region) to capture it.

```bash
cd ~/Desktop/Scabale-System-Go/Building-Scalable-Systems-CS6650/midterm-mastery/part2
mkdir -p screenshots
```

## From local results (any time)
```bash
open charts/exp1_loss_vs_size.png        && screencapture -w screenshots/01-exp1-loss-vs-size.png
open charts/exp1_retry_volume.png        && screencapture -w screenshots/02-exp1-retry-volume.png
open charts/exp3_autoscale_timeline.png  && screencapture -w screenshots/03-exp3-autoscale-timeline.png
open results/exp3.html                   && screencapture -w screenshots/04-exp3-locust-report.png
open results/invalid-exp2-locust-stalled/exp2_inflight4_u40.html && screencapture -w screenshots/05-exp2-inflight4-u40-locust.png
# terminal evidence: run each in a terminal, then capture that terminal window
column -s, -t results/exp1_summary.csv
column -s, -t results/exp1_s0_r0_backend_stats.csv results/exp3_backend_stats.csv
screencapture -w screenshots/06-backend-stats-terminal.png
```

## AWS console (only while the stack is still deployed)
```bash
open "https://us-west-2.console.aws.amazon.com/ecs/v2/clusters/file-transfer/services/gateway/events?region=us-west-2"
screencapture -w screenshots/07-ecs-gateway-scaling-events.png
open "https://us-west-2.console.aws.amazon.com/ecs/v2/clusters/file-transfer/services/backend/health?region=us-west-2"
screencapture -w screenshots/08-ecs-backend-cpu.png
open "https://us-west-2.console.aws.amazon.com/cloudwatch/home?region=us-west-2#alarmsV2:"
screencapture -w screenshots/09-cloudwatch-target-tracking-alarms.png
open "https://us-west-2.console.aws.amazon.com/cloudwatch/home?region=us-west-2#logsV2:logs-insights"
# paste the gateway FETCH_STATS query from scripts/logs_insights.sh, choose log group /ecs/file-transfer-gateway, time range: last 1 day, Run
screencapture -w screenshots/10-logs-insights-fetch-stats.png
```

Tear down when finished (this deletes the stack and the test files under `tmp/`):
```bash
cd ~/Desktop/Scabale-System-Go/file-transfer-playservice && scripts/destroy.sh --yes
aws s3 rb s3://file-transfer-641970357258-us --force
```

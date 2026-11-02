# Changes I made to the service (fork: utkarshsharma19/file-transfer-playservice, branch `experiments`)

The service itself was forked into its own repo (not this 6650 repo), as required. I did **not**
change any behaviour. I only added logging so the experiments could see what happens inside a
download, then rebuilt and pushed both images with `scripts/deploy.sh`.

| File | Change | Why |
|---|---|---|
| `transfer-client/udp/udp-client.go` (runs inside the **gateway**) | `Fetch` now writes one line per download: `FETCH_STATS file=… chunks=… open_attempts=… retry_rounds=… retys_sent=… missing_final=… dur_ms=… ok=… outcome=…`. The old "Missing chunks [list of every ID]" line now prints only the count and round number. | Lets CloudWatch Logs Insights count how many OPEN attempts, RETY rounds and RETY datagrams each download needed, split by file and outcome. The old line printed thousands of chunk IDs per round for large files, which flooded the logs. |
| `transfer-server/protocol/server.go` (the **backend**) | Atomic counters, printed every 10 s as `BACKEND_STATS opens=… retys=… source_fetches=… avg_fetch_ms=… datagrams_sent=… overload_drops=… queue_high=… queue_cap=…`. S3 reads go through a small `timedFetch` wrapper. | Shows the cost on the server side: how many times the whole file is read from S3 (every RETY reads it again), how many datagrams the job queue drops when full ("Overloaded"), and how close the queue (capacity `workers*4` = 32) gets to full. |

| `transfer-server/Dockerfile`, `transfer-frontend/Dockerfile` | Build stage is now `FROM --platform=$BUILDPLATFORM golang:1.27` and compiles with `GOARCH=$TARGETARCH`. | On my Apple-silicon Mac, `docker build --platform linux/amd64` ran the Go compiler under QEMU emulation and it crashed (`fatal error: found pointer to free object`). Compiling natively and cross-compiling to amd64 produces the same linux/amd64 image Fargate needs. Build-only change. |

`gofmt`, `go vet` and the full `go test -race ./...` suite still pass after the change.

No other files changed. The settings I varied (loss, `max_inflight`, scaler min/max) were all
passed as Terraform variables with `scripts/deploy.sh --var …`, so no `.tf` file was edited.

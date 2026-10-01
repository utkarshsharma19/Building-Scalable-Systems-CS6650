# HW1b: Rocking AWS

## Setup summary

- **Instance:** `Lab_EC2` (`i-0ccf3188f340d2677`), t2.micro, Amazon Linux 2023 (x86-64), us-west-2c (Oregon)
- **Addresses:** public IP 16.145.225.51, private IP 172.31.5.217
- **Security group:** SSH (22) and Custom TCP 8080, both restricted to My IP
- **IAM instance profile:** LabInstanceProfile
- **Key pair:** `Utkarsh_Lab_Key` (RSA `.pem`, stored in `~/.ssh` with `chmod 400`)
- **Code change:** `router.Run("localhost:8080")` → `router.Run("0.0.0.0:8080")`
- **Binary:** `albums-server`, run in the background with `nohup ./albums-server > server.log 2>&1 &` so it survives SSH disconnects
- **Cleanup:** terminated the instance after testing to conserve the Learner Lab budget

All times below are UTC (my local time is PDT, UTC−7).

---

## Part I: Running the Go-Gin server on EC2

![curl from my laptop to EC2](screenshots/curl.png)
![server running on EC2](screenshots/server_on_ec2.png)

### How I deployed it

1. **Cross-compiled on my Mac (Apple Silicon):**
```bash
   GOOS=linux GOARCH=amd64 go build -o albums-server main.go
```
   This produced a ~12 MB Linux x86-64 binary.

2. **The upload failed.** `scp` and `rsync` of the binary repeatedly died with "Connection reset by peer" / "Broken pipe". Shrinking the binary with `-ldflags="-s -w"` didn't help, and neither did lowering my Mac's MTU to 1300. Interactive SSH and small files worked fine; my home network was cutting off large transfers.

3. **Workaround: built on the instance.** I copied only the source (`main.go`, `go.mod`, `go.sum`, about 10 KB total) with `scp`, then compiled on the instance:
```bash
   sudo dnf install -y golang
   sudo dd if=/dev/zero of=/swapfile bs=1M count=2048
   sudo chmod 600 /swapfile && sudo mkswap /swapfile && sudo swapon /swapfile
   cd ~/hw1b
   nohup env GOTOOLCHAIN=auto GOSUMDB=sum.golang.org GOPROXY=https://proxy.golang.org,direct \
     go build -o albums-server main.go > build.log 2>&1 &
```
   - Amazon Linux ships Go 1.26.8, but my `go.mod` requires 1.27.1, so `GOTOOLCHAIN=auto` lets Go download the right version.
   - Amazon Linux's Go package disables Go's checksum database (`GOSUMDB=off`), and Go refuses to download a toolchain without it, so I turned it back on for this build.
   - The first build attempt didn't complete on 1 GiB of RAM with no swap. Adding a 2 GiB swap file and running the build in the background under `nohup` fixed it.

4. **Ran and tested:** started the server with `nohup ./albums-server > server.log 2>&1 &`, opened port 8080 to My IP, and ran `curl -v` from my Mac.

### Observations

- `curl -v http://16.145.225.51:8080/albums` from my laptop returned `HTTP/1.1 200 OK` with `Content-Type: application/json; charset=utf-8` and a 382-byte body containing the three albums. The verbose output shows each stage: the TCP connection to port 8080, the request headers curl sent (`>` lines), and the response headers (`<` lines).
- Binding to `0.0.0.0` is essential. `localhost` accepts connections only from the machine itself. The public IP isn't actually on the instance: the internet gateway translates it 1:1 to the private IP 172.31.5.217. So internet traffic can only reach a server listening on all interfaces.
- The security group is a stateful firewall. Port 8080 needs an explicit inbound rule, or packets are silently dropped and requests time out. Replies are allowed back out automatically, so no outbound rule was needed. I restricted both SSH and 8080 to My IP because my laptop was the only client (least privilege).
- Cross-compiling itself worked: one command produced a statically linked Linux/amd64 binary that needs no Go installation on the server, only a matching OS and CPU architecture. The problem was getting 12 MB through my network, not the build.
- Building on the instance showed how small a t2.micro is. The compile needed swap to finish, and CloudWatch recorded it as a CPU spike peaking at about 86% (Part III). Cross-compiling on a fast machine and shipping the binary is the better pattern; building on the server was my fallback.
- In debug mode, Gin prints warnings worth acting on in production:
  - **Switch to release mode** (`GIN_MODE=release`). This reduces debug output and stops Gin printing its internal route table at startup.
  - **Stop trusting all proxies.** By default Gin believes any client-supplied `X-Forwarded-For` header as the client's IP, so a client could spoof the IP in my logs. With no proxy in front of this server, `router.SetTrustedProxies(nil)` makes Gin use the real TCP connection address.

### Prep questions

**What is a VM, and how is running on one different from running locally?**
A VM is a software-defined computer. A hypervisor divides one physical server into isolated machines, each running its own OS kernel, which also lets multiple operating systems run side by side on the same hardware. Running on the VM differed from running locally in several ways:
- a different OS and CPU architecture, hence cross-compiling;
- access only over the network via SSH;
- traffic passing through a firewall and NAT;
- small, metered resources on hardware shared with other customers;
- observing the machine from outside (CloudWatch) instead of looking at it directly.

**What if my IP address changes?**
SSH (and port 8080, since it was restricted to My IP) would hang and time out, because the security group silently drops packets from the new address. I would update the inbound rules to my new IP. My network reports an IPv6 address by default, so I used `curl -4 ifconfig.me` to see the IPv4 address that AWS matches against.

**How does an Elastic IP help, and what does it cost?**
The auto-assigned public IP is released when the instance stops and replaced when it starts (a reboot keeps it). Learner Lab also stops instances whenever a 4-hour session ends, so the IP changes almost every session.

An Elastic IP is a static public IPv4 address owned by the account rather than the instance. It survives stop/start and can be remapped to another instance in seconds.

AWS charges $0.005/hour (about $3.60/month) for every public IPv4 address, attached or idle. An auto-assigned IP costs the same while the instance runs, so the real difference is that an Elastic IP keeps billing while the instance is stopped. It must be released when no longer needed.

**What level of service does t2.micro provide, and why care?**
t2.micro provides:
- 1 vCPU and 1 GiB RAM;
- EBS-only storage and "low to moderate" network performance;
- about $0.0116/hour.

It is burstable: its baseline is 10% of the vCPU, it earns CPU credits to burst above that, and it is throttled back to the baseline when credits run out.

Specs determine what the workload is bottlenecked on and how predictable its performance is. A burstable instance can look fine in a short test but fall short under sustained load in a larger system. I hit these limits directly, compiling the server needed a 2 GiB swap file to fit in memory and drove CPU to about 86%.

**What was different between GCP and AWS?**
Both homeworks ran the server on a VM, but AWS made me handle more of the setup explicitly: creating and protecting a key pair, choosing an IAM instance profile, opening port 8080 in a security group, building for the right architecture, and getting the program onto the machine. With my network dropping large uploads, that last step meant building on the instance. That means more control and visibility, but more setup and more ways to break. It's a great learning experience, useful when you have more operational needs, and it keeps things very transparent.

**What testing did I do?**
I sent `curl -v` requests from my laptop to `http://16.145.225.51:8080/albums` and confirmed a 200 response with the expected JSON. That verifies the binary, the `0.0.0.0` bind address, and the security group rule together. Part II then measured performance under sequential load, and Part III checked the instance from CloudWatch.

---

## Part II: Performance testing


| Metric | 60-second run | 10-minute run |
|---|---|---|
| Total requests | 1,149 (~19/s) | 11,405 (~19/s) |
| Mean | 52.12 ms | — |
| Median (p50) | 51.06 ms | 52.12 ms |
| p95 | 62.68 ms | 63.34 ms |
| p99 | 87.88 ms | 77.47 ms |
| Max | 208.80 ms | 218.71 ms |

**How the test works.** The script is a closed-loop test with one request in flight: each GET is sent only after the previous one returns. That gave about 19 requests/second, consistent with 60,000 ms ÷ 52.12 ms ≈ 1,151 requests. Because `requests.get()` creates a new session each call, every request also opens a new TCP connection. I ran the test twice: the required 60 seconds, and 10 minutes so CloudWatch could register it.

**1. Distribution shape.**
Yes, there is a very small long tail:
- Most requests fell between about 38 and 65 ms, peaking near 50 ms.
- A thin tail of outliers stretched out to 208.80 ms.
- The mean is above the median (52.12 vs. 51.06 ms), which is the numerical signature of right skew.

Defining "slow" as more than twice the median (about 102 ms), only about 0.4% of requests were slow (roughly 5 of 1,149, counted from the scatter plot).

**2. Consistency over time.**
Response times were stable across the whole 60-second run, with no upward drift and no periodic pattern. The 10-minute run showed the same flat band, so nothing degraded over time: no buildup of load and no CPU credit exhaustion. The first request took about 119 ms, a warm-up cost (Python and connection initialization). The remaining spikes are isolated and randomly distributed, which points to network jitter or occasional packet loss rather than a problem on the server. Note that the scatter plot uses request number on the x-axis, not elapsed time, so a stall would appear as a single dot rather than a gap.

**3. Median vs. 95th percentile.**
The median is 51.06 ms and p95 is 62.68 ms, a gap of only 11.62 ms: 95% of requests finished within about 12 ms of the typical one. The tail widens beyond that: p99 is 87.88 ms (about 1.7× the median) and the max is 208.80 ms (about 4× the median). The mean (52.12 ms) sits just 1.06 ms above the median, which confirms the skew is slight: a handful of outliers, not a heavy tail.

The 10-minute run shows why tail numbers need lots of samples:
- **Median and p95 barely moved** (52.12 and 63.34 ms).
- **p99 dropped** from 87.88 to 77.47 ms. With 1,149 samples, p99 rests on only about 11 data points, so a few unlucky spikes pulled it up. With 11,405 samples it rests on about 114, a much more stable estimate.
- **The max grew** to 218.71 ms, because more requests means more chances to hit a rare slow event.

The tail still matters at scale: if one page fans out to 100 backend calls, each with a 1% chance of being slow, about 63% of page loads hit at least one slow call (1 − 0.99^100).

**4. Infrastructure impact.**
I ran a bare binary rather than a container, but the same reasoning applies. A t2.micro is a burstable, single-vCPU VM on shared hardware, so variance can come from:
- other tenants' workloads stealing CPU time (noisy neighbors);
- older network virtualization and "low to moderate" networking;
- garbage collection, logging, and request handling all competing for one vCPU. Gin logs every request to stdout, which in my case went to `server.log`.

At one request in flight, though, the server was idle more than 99% of the time and CPU credits were never a factor. Most of the variability I measured came from the network, not the instance.

**5. Scaling to 100 concurrent users.**
Go itself handles 100 connections easily, with one lightweight goroutine per connection. But 100 closed-loop clients at about 50 ms each would offer roughly 2,000 requests/second, versus about 19 in my test. That would push the single vCPU far above the t2.micro's 10% baseline.
- **CPU credits** would drain, and once they ran out the instance would be throttled to 10% of a vCPU, causing a sharp performance drop.
- **Queueing** makes latency grow nonlinearly as utilization approaches 100%, and the tail degrades first.
- **Saturation:** once the server is maxed out, Little's Law sets latency: response time ≈ requests in flight ÷ maximum throughput.
- **The load generator would also break.** A new TCP connection per request would pile up sockets in TIME_WAIT. I would use keep-alive and a real load tool (hey, wrk, k6, Locust), ideally run from inside AWS.
- **Concurrency exposes a data race.** `postAlbums` appends to a global slice without a lock while GETs read it.
- **Horizontal scaling needs shared state.** Scaling out behind a load balancer would require moving the albums into shared storage, or each instance would serve its own diverging copy.

**6. Network vs. processing.**
Mostly network. Each request pays one round trip for the TCP handshake and one for the HTTP request and response, so a median of about 51 ms is consistent with a round-trip time of about 25 ms to Oregon. Gin's own per-request log line reports server-side handling time, which for this endpoint is well under a millisecond, so server processing is a tiny share of the total.

To investigate further, I would:
- compare Gin's logged server time with the client-measured time for the same requests;
- use `curl -w` to split connect time from time-to-first-byte;
- rerun the test from inside AWS (or on the instance itself), so whatever latency disappears was network;
- reuse connections with `requests.Session()`, which should cut about one round trip;
- capture packets to match outliers to retransmissions.

**Limitations of this test.**
- `time.time()` is wall-clock time; `time.perf_counter()` is more accurate for measuring intervals.
- Failed or timed-out requests are never recorded, so the worst outcomes are missing from the percentiles.

---

## Part III: CloudWatch

**Observations.** Baseline CPU sat around 1%.
- **The 86% hump (08:30–08:40 UTC)** was installing Go and compiling the server on the instance, not load testing.
- **The 60-second test (around 08:56–08:57 UTC)** is lost in the flat line at this scale.
- **The 10-minute test (about 09:00–09:10 UTC)** appears only as a small bump in the zoomed view, for two reasons:
  - basic monitoring reports 5-minute averages, which does not give right answers for tests since it dilutes the answer when ran on a larger system compared to a smaller one;
  - about 19 lightweight requests/second costs well under 1% of a vCPU.

The graph shows the machine was busy both times, but nothing in it says that one spike was a compiler and the other was serving requests.

**What CloudWatch can and can't tell us.** CloudWatch's default EC2 metrics tell us the instance is up and working: CPU utilization, network traffic, CPU credit balance, and status checks, averaged over 5-minute windows from the hypervisor's point of view. They can't tell us whether the service is answering correctly or how fast. There is no request latency, no p50 or p99, no error rate, no memory usage, and the 5-minute averages hide short spikes. Busy is not the same as healthy, and healthy is not the same as fast: Part II measured what users experience, while CloudWatch measured what the machine experienced.
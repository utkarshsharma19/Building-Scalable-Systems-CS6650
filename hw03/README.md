# HW3: Threading and Load Tests

I ran everything on an M1 MacBook (8 cores) with Go 1.27. Docker had 4 CPUs.

Part I (the Lamport paper review) is posted on Piazza in the hw3 folder.

## Part II: Thread experiments

The code is in `part2/`. Run each experiment with `go run ./part2/<name>`.

### Atomicity

50 goroutines each add 1 to two counters 1,000 times, so both should end at 50,000. The atomic counter was always 50,000. The plain counter was wrong in all 10 runs, landing between about 11,600 and 19,300.

![](screenshots/p2-01-atomic-vs-plain.png)

`plain++` is really three steps: read, add, write. Two goroutines can read the same value, both add 1, and both write it back, which loses one update. `atomic.Add` does all three as one step that can't be interrupted.

Running with `-race` turns on Go's race detector. It pointed straight at the `plain++` line and reported 2 data races.

![](screenshots/p2-02-atomic-race-detector.png)

### Collections

50 goroutines each write 1,000 keys into a shared map.

With a plain map, the program crashed every time with `fatal error: concurrent map writes`. Go detects two goroutines writing to a map at once and kills the program deliberately, because a map that is resized during a concurrent write can get corrupted.

![](screenshots/p2-03-plain-map-crash.png)

Then I made it safe three ways and took the mean of 3 runs. All three ended with len = 50,000.

- Mutex: 8.47 ms
- RWMutex: 8.53 ms
- sync.Map: 3.91 ms

![](screenshots/p2-04-maps-write-only.png)

The Mutex is correct but slow, because every write waits for one lock. That makes the work serial again. Switching to RWMutex changed nothing: every operation here is a write, and writes still need the exclusive lock. sync.Map was about twice as fast. Each goroutine writes its own keys and never reads them back, which is one of the cases sync.Map is built for.

To see what happens when reads dominate, I re-ran it with 90% reads (`-reads=90`):

- Mutex: 5.90 ms
- RWMutex: 3.31 ms
- sync.Map: 2.09 ms

![](screenshots/p2-05-maps-90pct-reads.png)

![](charts/maps.svg)

Now RWMutex is almost twice as fast as Mutex, because many readers can hold the lock at once.

The trade-offs:
- A Mutex is simple and works for anything, but serializes everything.
- An RWMutex only helps when there are lots of reads.
- sync.Map is fastest for this pattern, but it loses type safety, has no length function (I had to count with Range), and can't update several keys atomically.

### File access

Writing 100,000 lines took 168.6 ms unbuffered and 10.0 ms buffered, about 17× faster.

![](screenshots/p2-06-file-buffered-vs-unbuffered.png)

Each unbuffered `f.Write` is a system call into the kernel, so that's 100,000 trips. `bufio.Writer` collects lines in a 4 KB buffer in memory and only calls into the kernel when the buffer is full. The trade-off is safety: if the program crashes before `Flush`, the buffered lines are lost. Databases face the same choice when they decide how often to flush to disk.

### Context switching

Two goroutines pass a signal back and forth 1 million times. With `GOMAXPROCS(1)` each switch took about 105 ns. With all 8 cores it took about 120 ns.

![](screenshots/p2-07-context-switch.png)

One thread was faster. With a single thread, Go's scheduler swaps goroutines without involving the OS, and the data stays in one core's cache. With multiple threads the goroutines can land on different cores, so each hand-off may need to wake an OS thread and move data between cores. More cores don't help here, because only one side can work at a time.

Switching goroutines is the cheapest kind of switch. OS threads and processes cost more because the kernel has to save more state. Containers are just processes, so they cost about the same as processes. VMs are the most expensive, because the hypervisor has to save the entire guest machine's CPU state.

## Part III: Load testing with Locust

The code is in `part3/`. I reused my HW2 albums server, but swapped its slice for a map protected by an RWMutex. The old version appended to a shared slice from concurrent requests, which is the same race as Part II. The locustfile sends GET and POST requests at a 3:1 ratio.

```
cd part3
docker compose up --build                       # 1 worker
docker compose up --build --scale worker=4      # 4 workers
LOCUSTFILE=locustfile_fast.py docker compose up --build --scale worker=4
```

None of the runs had any failures.

### 1 worker, 1 user

![](screenshots/p3-01-1w-1u-stats.png)
![](screenshots/p3-02-1w-1u-no-failures.png)

POST was a little slower than GET (median 0.28 ms vs 0.25 ms). A POST has to parse a JSON body, take the exclusive write lock, and insert into the map. A GET only takes the shared read lock and looks up a key.

In a real album store, reads are far more common than writes. That's why I used a map, for fast lookups by ID, with an RWMutex so reads can run in parallel.

### 1 worker, 50 users

This got 4,238 requests per second, with a median of 8 ms.

![](screenshots/p3-03-1w-50u-stats.png)
![](screenshots/p3-04-1w-50u-charts.png)

Going from 1 to 50 users barely raised throughput, but latency went from 0.25 ms to 8 ms. The Locust worker itself was the bottleneck: it logged that its CPU was above 90%.

![](screenshots/p3-05-1w-50u-worker-cpu-warning.png)

### 4 workers (Amdahl's Law)

This got 11,788 requests per second, with a median of 2 ms.

![](screenshots/p3-06-4w-50u-stats.png)
![](screenshots/p3-07-4w-50u-charts.png)
![](screenshots/p3-08-4w-50u-no-failures.png)

Four times the workers gave about 2.8× the throughput, not 4×. `docker stats` shows why: the workers and the server together were using all 4 CPUs, so every extra worker took CPU away from the server.

![](screenshots/p3-09-4w-50u-docker-stats.png)

The map also adds a small serial part. Every POST takes the exclusive lock, so writes happen one at a time and reads wait behind them. Adding more workers can't fix that part, which is what Amdahl's Law describes.

### FastHttpUser

With FastHttpUser, 1 worker reached 13,118 requests per second and 4 workers reached 41,138. That's about 3× more than HttpUser in both cases. A single FastHttpUser worker beat four HttpUser workers.

![](screenshots/p3-10-fast-1w-50u-stats.png)
![](screenshots/p3-11-fast-1w-50u-charts.png)
![](screenshots/p3-12-fast-4w-50u-stats.png)
![](screenshots/p3-13-fast-4w-50u-charts.png)

![](charts/locust_rps.svg)

HttpUser uses the `requests` library, which is pure Python and does a lot of work per request. FastHttpUser uses a C-based HTTP client, so each request costs much less CPU. Locust users are green threads, so switching between them is already cheap. FastHttpUser shrinks the work each one does between switches. The main lesson: a load test measures the load generator too, so check its CPU before trusting the numbers.

## Part IV: Concurrent server design

The provided code is in `part4/go-rpc/`. I left it unchanged.

### Why it doesn't scale

`main` calls `Accept()` once and handles that one client. When that client disconnects, the server exits.

![](screenshots/p4-01-server-exits-after-one-client.png)

A second client can connect, but nobody ever reads its requests.

![](screenshots/p4-02-second-client-dump-no-reply.png)

While walking through the code I also found a few bugs that crash the whole server:
- `GET` with an unknown ID sends the error message but has no `return`, so it then dereferences a nil pointer and crashes.
- A bare `GET` or `STORE` crashes with an index out of range.
- If the port is already taken, the `Listen` error is printed but ignored, and the server crashes.
- The `-port` flag is parsed but never used.

![](screenshots/p4-03-get-99-error-then-disconnect.png)
![](screenshots/p4-04-get-99-nil-pointer-panic.png)
![](screenshots/p4-05-listen-error-unhandled-panic.png)
![](screenshots/p4-06-port-flag-ignored.png)

### My design

Loop on `Accept()` and handle each connection in its own goroutine. Goroutines are cheap: Part II measured a switch at about 100 ns. While a client is idle, its goroutine is just parked waiting on the socket. I would cap the number of connections, set a timeout so idle clients get dropped, and add a `recover()` in each goroutine so one bad request only kills that client's connection.

**Data types.** The `Album` type stays the same, but the storage has to change. Right now it's a global slice with no lock, so two STOREs at the same time could get the same ID. I would put the slice behind an RWMutex. STORE takes the write lock for the whole check-then-insert, so duplicate checking and ID assignment happen together. GET, DUMP and SEARCH take the read lock. I would also add a map from album to ID so STORE doesn't have to scan the whole list. One rule matters a lot: copy the data under the lock and release it before sending anything over the network, so a slow client can't hold up everyone else.

**Scaling.** Reads run in parallel across cores, so throughput grows with more cores. Writes still go one at a time. That's the serial part from Amdahl's Law, so a write-heavy load will level off. Response times stay flat until the CPU or the write lock is saturated, then requests start to queue. To go beyond one machine you would need a shared database or sharding, because otherwise you get the HW2 Part IV problem of servers with different data.

**Starvation.** Every client has its own goroutine, and Go's scheduler is preemptive, so one client can't block the others. Go's RWMutex also stops letting new readers in once a writer is waiting, so a steady stream of GETs can't starve a STORE. And since nothing holds the lock while talking to the network, a slow client only slows itself down.

**Worst case.** Many DUMP or SEARCH requests on a very large store. Each one copies the whole list, which costs a lot of CPU and memory, and writers queue behind those long reads. To fix it I would:
- keep an immutable snapshot that writers replace, so readers don't need a lock at all,
- page the DUMP results,
- add a word index for SEARCH.

For write-heavy bursts, I would split the store into several shards, each with its own lock.

## What I learned

Shared data needs atomics or locks, or you silently lose updates. The right lock depends on how many reads and writes you have. System calls and context switches have real costs, and batching or lighter threads avoid them. Adding more workers doesn't scale linearly, because of serial parts and shared CPUs. A load test also measures the machine generating the load. Finally, a server that handles one client at a time, or that one bad request can crash, can't scale no matter how fast it is.

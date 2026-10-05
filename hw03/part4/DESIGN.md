# Part IV: Concurrent Server Design for KV_PROTOCOL

The provided code is in `go-rpc/` (unchanged, Oct 1 version with the client).

## 1. Code walkthrough

| File | Role |
|---|---|
| `server/server.go` | `main`: parse port, `net.Listen("tcp", ":8080")`, `Accept()` **once**, call `Handle_client(conn)`, then return |
| `server/kv_protocol_server.go` | `Handle_client`: loop `ReadString('\n')` → `process_message` → switch on the first word (STORE / GET / DUMP / SEARCH / CLOSE) → reply with `fmt.Fprintln(conn, …)` |
| `albums/album.go` | Storage: a package-level `var albums []Album` (3 seeded albums). Fields are unexported; IDs are slice indexes. `Store_album` does a linear `containsAlbum` scan for dedupe, then `append`. `Retrieve_all`/`Search_titles_with_phrase` return copies; `Retrieve_by_id` is a linear scan |
| `client/client.go` | Makeshift client: DUMP, GET 1, STORE, SEARCH Blue, DUMP, CLOSE on one connection |

Request flow: **TCP accept → one goroutine (main) → read line → parse → touch the global slice → write reply → read next line.**

### Why it can't serve more than one client

`main` calls `ln.Accept()` exactly once and handles that connection on the main goroutine. I verified this by running it:

- **A second client connects but is never served.** TCP connect succeeds because the kernel queues it in the listen backlog, but nobody calls `Accept()` again, so its `DUMP` got no reply after 3s.
- **When the first client closes, the process exits.** `Handle_client` returns, `main` returns, and the server is gone.
- **Throughput is capped at one client's think time.** The server spends most of its time blocked in `ReadString` waiting for one client to type, while every other client waits.

### Other bugs found while walking through (protocol-level, not structural)

| Where | Bug | Effect (verified where marked ✓) |
|---|---|---|
| `process_get_command` | no `return` after sending `ERR album not found` | `GET 99` dereferences a nil `*Album` and **panics, killing the server for everyone** ✓ |
| `process_message` | `strings.Split(msg, " ")[1]` with no length check | bare `GET` or `STORE` panics with index out of range (spec says reply `ERR Need to input an ID`) |
| `process_store_command` | no `return` after `ERR invalid album data format` | fewer than 3 parts panics; more than 3 is silently accepted |
| `process_store_command` | `Split(msg, " ")[1]` | a title with spaces (`STORE Kind of Blue:Miles:9.99`) is cut at the first space |
| `extract_port_number` | parses the port and then throws it away | `-port=9090` still listens on 8080 ✓ |
| `main` | `Listen` / `Accept` errors are printed but not handled | with port 8080 already taken, it prints "Failed to listen", then "Listening on port 8080", then panics on a nil `ln` at `server.go:42` ✓ |
| replies | GET omits `FOUND `; DUMP/SEARCH lines omit `id:`; unknown command text differs from spec | client can't map DUMP results back to IDs ✓ (client output shows `title:author:price`) |

In a concurrent server, the panics matter more: one malformed request from one client would take down every other client's session. A `recover()` per connection (see below) contains that.

## 2. Proposed design

### 2.1 Connection handling: goroutine per connection, bounded

```go
sem := make(chan struct{}, MAX_CONNS)          // e.g. 10,000
for {
    conn, err := ln.Accept()
    if err != nil { log.Print(err); continue }
    sem <- struct{}{}                          // backpressure when full
    go func() {
        defer func() { <-sem; recover() }()    // a panic kills one session, not the server
        conn.SetDeadline(time.Now().Add(IDLE_TIMEOUT))   // refreshed after each command
        Handle_client(conn, store)
    }()
}
```

- **Why a goroutine per connection rather than a fixed worker pool:** connections are long-lived and mostly idle (blocked in `ReadString`). A goroutine costs about 2–8 KB of stack, and a blocked read parks it in Go's netpoller (epoll/kqueue) without holding an OS thread. Part II's context-switch experiment measured about 100–150 ns per goroutine hand-off, so thousands of mostly idle clients are cheap. A fixed pool of N workers would let N idle clients block everyone else, which is the same problem as today.
- **Bounded:** the semaphore caps memory and file descriptors. Beyond the cap, new clients wait in the backlog instead of exhausting the process.
- **Deadlines:** an idle or malicious client that connects and never sends anything (slowloris) is disconnected after `IDLE_TIMEOUT` instead of holding a slot forever. A write deadline also protects against a client that never reads its DUMP response.

### 2.2 Storage: same data types, synchronized store

**Does the design change the data types?** `Album` stays exactly the same: the same fields, the same unexported encapsulation, the same accessors. What changes is the **storage**, because the global `var albums []Album` becomes shared mutable state the moment two goroutines touch it. That is the race from Part II: `Store_album` reads `len(albums)` and appends, so two concurrent STOREs can be assigned the **same ID** or lose an append.

```go
type Store struct {
    mu    sync.RWMutex
    byID  []Album              // ID == index, append-only, so GET is O(1)
    index map[albumKey]uint    // title|artist|price -> id, so STORE dedupe is O(1)
}
```

| Command | Lock | Cost (today → proposed) |
|---|---|---|
| `STORE` | `Lock`, held across check **and** insert (dedupe + ID assignment must be one atomic step) | O(n) scan → O(1) map lookup |
| `GET id` | `RLock` | O(n) scan → O(1) index (`strconv.Atoi(id)`, bounds check) |
| `DUMP` | `RLock` only to **copy** the slice, then release **before** writing to the socket | O(n), unchanged |
| `SEARCH` | `RLock` to copy or filter, release, then send | O(n), unchanged (see 2.4) |

The key rule is **never hold the lock while doing network I/O.** A slow client reading a 10 MB DUMP must not block every STORE. `Retrieve_all` already copies, so we just make sure the send happens after unlock.

RWMutex rather than Mutex or sync.Map: the workload is read-mostly (GET/DUMP/SEARCH vs STORE). Part II showed RWMutex beating Mutex once reads dominate (3.2 ms vs 5.4 ms at 90% reads). sync.Map was faster still, but it can't do an atomic check-then-insert across two structures (`byID` + `index`), and it isn't a good fit for ordered append-only IDs.

### 2.3 How it scales as requests increase

- **Concurrency:** throughput grows with cores, because each connection's parse and format work runs in parallel on `GOMAXPROCS` threads, and GET/DUMP/SEARCH readers run in parallel under `RLock`.
- **Serial fraction (Amdahl):** every STORE takes the exclusive lock, so the write rate is capped at one writer at a time, and speedup approaches 1 / (serial fraction). With a 3:1 read/write mix this is acceptable. A write-heavy mix would hit the ceiling.
- **Response rate:** latency stays flat until the CPU or the write lock saturates, then it rises as requests queue. Throughput levels off instead of collapsing, because the connection cap provides backpressure.
- **Beyond one machine:** the store is in memory in one process. To scale out, put several stateless servers behind a load balancer with a shared store, or shard by ID (`id % N`). This is the HW2 Part IV lesson: two instances with private memory disagree.

### 2.4 Starvation

- **Between clients:** each connection is its own goroutine, and Go's scheduler is preemptive (since Go 1.14), so a client running an expensive SEARCH can't monopolize a CPU. Unlike today's design, an idle client blocks only its own goroutine.
- **Writers vs readers:** Go's `RWMutex` blocks **new** readers once a writer is waiting, so a steady stream of GETs cannot starve a STORE. The writer waits only for readers already inside.
- **Readers vs writers:** writes hold the lock for O(1) (a map insert plus an append), so readers wait microseconds.
- **Slow consumers:** because I/O happens outside the lock, a client that reads slowly only delays itself. Write deadlines eventually drop it.

### 2.5 Worst case and fixes

| Scenario | Why the design allows it | Fix |
|---|---|---|
| **Huge store + many concurrent DUMP/SEARCH** | each is O(n) CPU and memory (a copy per request) and O(n) bandwidth | stream from a shared immutable snapshot instead of a per-request copy: copy-on-write with `atomic.Pointer[[]Album]`, so readers take no lock at all. Add paging (`DUMP offset limit`) and an inverted index (word → ids) for SEARCH |
| **Copy under RLock while writers queue** | a long O(n) copy holds the RLock, a writer queues, and then all new readers queue behind it, causing a latency spike | same COW snapshot: readers never lock, and writers swap a pointer |
| **Write-heavy burst** | the single write lock is the serial fraction | shard the store into K maps with K locks (`hash(key) % K`), with IDs from an `atomic.Uint64` counter |
| **Connection flood or idle sockets** | goroutines are cheap but not free | semaphore cap, idle and write deadlines, per-IP connection limits |
| **One malformed request** | the existing panics (GET 99, bare GET) | `recover()` per connection, and fix the missing `return`s and length checks |

## 3. If implemented: how I'd test it

Write a Locust `User` (or a Go client) that opens a TCP connection per user and sends a 3:1 GET:STORE mix plus occasional DUMP, using the same 1-worker and 4-worker setup as Part III. I'd compare against the original server, which would serve exactly one user and drop the rest.

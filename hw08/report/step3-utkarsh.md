## Utkarsh Sharma's analysis

### Part 0: Pre-analysis data verification

`scripts/verify_combined.py` checks that both files have exactly 150 operations, 50 of each type, all in the required format, and `scripts/analyze.py` merges them into `results/combined_results.json` with a `database` field on every record (300 records). Every table below is computed only from that file.

{figure:verify|screenshots/combined-verification.png|Output of verify_combined.py: 150 operations in each file, 50 of each type, and 300 records in combined_results.json}

### Part 1: Performance comparison

Data source: `combined_results.json`

| Metric | MySQL | DynamoDB | Winner | Margin |
|---|---|---|---|---|
| Avg Response Time (ms) | 88.72 | 91.79 | MySQL | 3.07 ms (3.3%) |
| P50 Response Time (ms) | 86.03 | 89.70 | MySQL | 3.67 ms (4.1%) |
| P95 Response Time (ms) | 97.69 | 99.49 | MySQL | 1.80 ms (1.8%) |
| P99 Response Time (ms) | 130.65 | 130.56 | DynamoDB | 0.09 ms (0.1%) |
| Success Rate (%) | 100 | 100 | Tie | 0 |
| Total Operations | 150 | 150 | | |

| Operation | MySQL Avg (ms) | DynamoDB Avg (ms) | Faster By |
|---|---|---|---|
| CREATE_CART | 88.11 | 95.07 | MySQL, 6.96 ms |
| ADD_ITEMS | 92.51 | 90.77 | DynamoDB, 1.74 ms |
| GET_CART | 85.55 | 89.54 | MySQL, 3.99 ms |

{figure:step3|charts/step3-comparison.png|My 150 operation results from combined_results.json. The axes don't start at 0, because about 86 ms of each request is the network}

MySQL wins most rows, but every margin is under 7 ms while about 86 ms of each request is the network from my laptop, and the two tests ran about 45 minutes apart from the same laptop. The request logs show what each database itself did in these exact 150 requests: create 3.09 ms on MySQL vs 4.67 ms on DynamoDB, add 9.24 vs 3.90 ms, and get 2.20 vs 3.03 ms. So MySQL is faster at simple inserts and primary-key reads, and DynamoDB is faster at adding an item, because that's one UpdateItem instead of a transaction with a commit. That's the same pattern Eason and Shikha found.

Under 100 users (same Locust session on both, from my STEP I and II sections), the median and p95 were the same, 98 and 110 ms, but MySQL's p99 was 160 ms vs 120 ms for DynamoDB. MySQL's difference comes from requests waiting for a pool connection (1.1% of requests, about 40 ms each).

#### Consistency model impact assessment

- *How often were there delays?* Never through the API on either database: 0 stale reads in 200 checks on each. MySQL can't have any, because the one primary is always up to date. DynamoDB only showed stale reads when my in-region probe read microseconds after the write: 17 of 10,000 (0.17%), and every one was fresh again within 22.5 ms.
- *What's most affected?* Code that writes and then reads right away inside the server, and read-modify-write. I avoided read-modify-write on both: MySQL locks the cart row in a transaction, and DynamoDB uses one atomic update. Both ended at exactly 200 when 10 clients added 200 items at once.
- *Designing for each:* with MySQL (ACID) I can use a transaction and every read sees the last commit. With DynamoDB (eventual) I'd use conditions and atomic updates instead of reading first, return the new item from the write, and use a strongly consistent read (twice the read cost) where it matters, like checkout.
- *User experience:* a stale cart would show an item missing right after adding it. In my data that window is at most 22.5 ms, shorter than one network round trip, so a person clicking around won't see it. At checkout it could charge for the wrong cart, so that's where I'd pay for a strong read.

### Part 2: Resource efficiency analysis

- *Connection overhead vs managed scaling:* MySQL needed a pool sized against `max_connections` (61 on db.t3.micro). With 10 per task, 2 tasks used 20, and 1.1% of requests still waited. Each new task adds 10 connections, so 6 tasks would hit the limit, and scaling the service means scaling the database or adding RDS Proxy. DynamoDB has no connections to manage, and on-demand absorbed about 340 writes/s with 0 throttles without my setting any capacity.
- *CPU in my service:* the DynamoDB runs used more ECS CPU (about 30% average) than MySQL (about 17%) for the same load. Every DynamoDB call is a signed HTTPS request with JSON, while MySQL uses a binary protocol on connections that stay open. So DynamoDB moves some work into the service.
- *Predictability:* MySQL was steady: RDS CPU 24 to 30%, exactly 20 connections and about 446 write IOPS for the whole run, so the remaining headroom is easy to see. DynamoDB's latency was just as steady (GetItem about 1.1 ms, UpdateItem about 2.8 ms every minute), and its cost is exact per request: 4 write units to 1 read unit, about 20,150 and 5,040 a minute.
- *Capacity planning:* with MySQL I have to choose the instance size, pool size and task count together, and RDS bills every hour even when nobody is shopping. With DynamoDB on-demand there's nothing to plan, and the cost follows the requests: my whole STEP II session was about 14 cents.
- *Operational complexity:* MySQL needed private subnets, a subnet group, a security group, a password, the schema at startup, pool settings and a startup retry while RDS boots, and it took about 10 minutes to create. DynamoDB was one table resource that was ready in seconds. The work moved into the code instead (attribute format, random IDs, conditions), and the two stores ended up about the same size, 230 lines for MySQL and about 220 for DynamoDB.

### Part 3: Real-world scenario recommendations

- **Scenario A, Startup MVP: DynamoDB.** At 100 users a day both are the same speed from the client (88.72 vs 91.79 ms), so speed doesn't decide it. DynamoDB costs nothing when idle (14 cents for my whole test session), while RDS bills every hour. It was also less infrastructure for one developer: one table instead of RDS, private subnets, a security group, a password and a schema.
- **Scenario B, Growing Business: MySQL.** 10K users a day is far below the about 490 requests/s my smallest RDS handled at 24 to 30% CPU. Feature growth means new queries: in MySQL, customer history only needed an index, while in DynamoDB it needed a GSI planned ahead. 5 developers can share SQL more easily than a key design.
- **Scenario C, High-Traffic Events: DynamoDB for carts.** On-demand absorbed my load with 0 throttles, and about 20,000 cart IDs per run were spread with no hot key. MySQL hits its connection limit first: 2 tasks already used 20 of 61 connections, and even at that load the pool waits pushed p99 to 160 ms vs 120 ms. I'd warm the table up before the event and give the tasks more CPU, since DynamoDB used more service CPU.
- **Scenario D, Global Platform: DynamoDB for carts and sessions, MySQL for orders.** Cart access is all by one key and spread evenly, which is what DynamoDB global tables replicate across regions. Orders and payments still need transactions, and a MySQL primary lives in one region.

### Part 4: Evidence-based architecture recommendations

**Shopping cart winner: DynamoDB.** The 150 operation test was close to a tie (3.07 ms on an 86 ms network), so I didn't pick it from that test. I picked it because under 100 users it had the same median and p95 as MySQL, a lower p99 (120 vs 160 ms), no pool to size and no waits, and it added items faster (3.9 vs 9.2 ms in the service), which is the most common cart operation.

- *Response time advantage:* 3.07 ms average for MySQL in the 150 test, noise next to the network. 40 ms at p99 for DynamoDB under load.
- *Implementation complexity:* about the same code (230 vs 220 lines), but much less infrastructure for DynamoDB (one table vs RDS with subnets, a security group, a password and a schema).
- *Other factors:* DynamoDB costs per request and nothing when idle, and it needed 0 capacity settings. It does need more service CPU (30% vs 17%), and every new query has to be planned into the keys.

**When to choose MySQL instead:** when the cart needs to join other data (inventory, promotions, prices) in one transaction, when there are many questions nobody has thought of yet (reports, admin searches), or when traffic is steady and fits one instance. MySQL was faster for creates and gets in the service (3.09 and 2.20 ms vs 4.67 and 3.03 ms).

**Polyglot strategy:**

- *Shopping carts:* DynamoDB, based on my results above.
- *User sessions:* DynamoDB, looked up by session ID only, and TTL expires them for free, like my abandoned carts.
- *Product catalog:* MySQL, since search by category, price and filters needs flexible queries and indexes.
- *Order history:* MySQL, since payments need ACID and "a customer's orders by date" is exactly my `(customer_id, created_at)` index.

### Part 5: Learning notes

*What surprised me:* from my laptop the two databases were almost the same, because about 86 of 90 ms is the network. I also expected Eason's DynamoDB tail to come from the SDK pool, but pool 10 and pool 200 were identical for me. My tasks never got CPU-bound, and I now think the CPU was the cause. Stale reads were real but rare: 17 in 10,000 in-region and 0 through the API. Finally, the DynamoDB service used almost twice the CPU of the MySQL one.

*What failed initially:* my first AWS account was a member of an organization whose policy denied DynamoDB, ECS, ECR and load balancers, and admin permissions can't override that. My first in-service query window also overlapped another test (219 gets counted instead of 50). Tests through the ALB couldn't produce a stale read, so I had to add the in-region probe to see eventual consistency at all.

*Key insights:* I'd definitely choose MySQL when the data is relational, needs transactions across tables, or needs new queries later. I'd definitely choose DynamoDB when every access is by one key, traffic is spiky or unknown, or idle cost matters. For another student: measure the network alone (a `/health` call) and log the time inside the database, otherwise you're comparing your home connection, not the databases. Change one setting at a time when testing a theory, like the SDK pool size. Doing it hands-on showed me the database choice changes the tail and the operations work far more than the average response time.

*Clean up:* after the screenshots I ran `terraform destroy -auto-approve` (32 destroyed) and checked that no RDS instance, DynamoDB table, ECS cluster, load balancer, ECR repository, NAT gateway, VPC or log group was left.

## Utkarsh Sharma's results

All my runs are in my own AWS account (us-west-2), since my Learner Lab budget ran out. The code, Terraform, result files and screenshots are in `hw08/` in my repo. My service runs 2 Fargate tasks (256 CPU, 512 MB, ARM64) behind an ALB, the same task count as the rest of the team.

### 1. Terraform: DynamoDB table design

`terraform/dynamodb.tf` creates one table, `hw8-shopping-carts`, on-demand (`PAY_PER_REQUEST`):

- *Partition key:* `cart_id` (number), no sort key. Every endpoint reads or writes exactly one cart by its ID, so a sort key would add nothing.
- *Items embedded:* a map of product ID to quantity inside the cart item, so a cart is always one GetItem and an add is one UpdateItem. 50 items is under 1 KB, far below the 400 KB item limit.
- *GSI `customer_id-index`:* `customer_id` + `created_at`, projection ALL, for customer history (`GET /customers/{id}/shopping-carts`, newest first). It's the one query the cart ID can't answer.
- *TTL on `expires_at`:* carts are session data, so abandoned carts expire after 30 days, and TTL deletes are free.
- *Contributor Insights* to see the most accessed keys, and a *gateway VPC endpoint* so DynamoDB traffic stays on the AWS network.
- *Least-privilege task role:* GetItem, PutItem, UpdateItem, Query and DescribeTable on this table and its index only. My tasks run in public subnets with a public IP and accept traffic only from the ALB, so there's no NAT gateway.

This is the cart from my concurrency test, read back from AWS after 10 clients added 200 items to it at once:

```
{"cart_id": {"N": "1964531303"}, "customer_id": {"N": "7000"}, "items": {"M": {"1": {"N": "80"}, "2": {"N": "60"}, "3": {"N": "60"}}},
 "version": {"N": "201"}, "created_at": {"S": "2026-11-16T08:56:30.034Z"}, "updated_at": {"S": "...08:56:32.148Z"}, "expires_at": {"N": "1797411390"}}
```

### 2. API implementation

The same handlers serve both databases. `main.go` validates input and maps errors to status codes, and `dynamo.go` and `mysql.go` implement one `cartStore` interface, picked with `-var database=dynamodb|mysql`.

| Endpoint | DynamoDB operation | Responses |
|---|---|---|
| POST /shopping-carts | PutItem, random cart ID, `attribute_not_exists(cart_id)` | 201, 400 |
| GET /shopping-carts/{id} | GetItem by partition key, eventually consistent (`?consistent=true` for a strong read) | 200, 400, 404 |
| POST /shopping-carts/{id}/items | one UpdateItem (below) | 204, 400, 404, 409 when a 51st product is added |

The add is one atomic expression:

```
SET #items.#pid = if_not_exists(#items.#pid, :zero) + :q, updated_at = :now, version = version + :one
CONDITION attribute_exists(cart_id) AND (attribute_exists(#items.#pid) OR size(#items) < :max)
```

`ReturnValuesOnConditionCheckFailure = ALL_OLD` tells the two failures apart: no old item means a 404, an old item means the cart already has 50 products, a 409. So the 50-item limit is enforced inside DynamoDB with no read first. Throttling (`ProvisionedThroughputExceeded` / `ThrottlingException`) becomes a 503, a timeout a 504, and anything else a 500 with the error only in the log. The SDK retries up to 3 times. All 16 of my functional checks (`scripts/smoke.sh`: 404 vs 400, malformed JSON, injection-style input, the 50-item limit) passed on AWS.

### 3. Performance testing results

#### The 150 operation test

`tests/load_test.py` sends 50 creates, then one add to each of those carts, then one get of each, one at a time over one keep-alive connection from my laptop, the same order as Tian. It took 13.8 s, and all 150 succeeded (`results/dynamodb_test_results.json`).

| Operation | n | Success | Avg | P50 | P95 | P99 | In the service, avg | Inside DynamoDB (CloudWatch), avg |
|---|---|---|---|---|---|---|---|---|
| create_cart | 50 | 50 | 95.07 ms | 90.69 ms | 102.95 ms | 182.13 ms | 4.67 ms | PutItem 3.19 ms |
| add_items | 50 | 50 | 90.77 ms | 89.84 ms | 95.26 ms | 104.42 ms | 3.90 ms | UpdateItem 2.93 ms |
| get_cart | 50 | 50 | 89.54 ms | 88.84 ms | 94.09 ms | 119.59 ms | 3.03 ms | GetItem 1.23 ms |
| all | 150 | 100% | 91.79 ms | 89.70 ms | 99.49 ms | 130.56 ms | | |

My numbers from the laptop are about 90 ms because I'm on the East Coast. `GET /health`, which doesn't touch the database, averaged 85.9 ms over 50 requests (`results/network_baseline.json`), so about 86 of the 90 ms is the network and DynamoDB is 1 to 3 ms of it ({fig:ddb150}). The test used exactly 100 write units and 25 read units: 100 writes at 1 unit each, plus 50 eventually consistent reads at half a unit each.

{figure:ddb150|charts/ddb-150-breakdown.png|Where the time goes in my 150 operation test: about 86 ms is the network to Oregon and 3 to 5 ms is the service}

#### 100 users: testing the SDK connection pool

Eason's DynamoDB run had a p95 of 170 ms vs 50 ms for MySQL. Eason's guess was the SDK keeping only 10 idle HTTPS connections per host, but it wasn't tested. I checked the SDK source (`DefaultHTTPTransportMaxIdleConnsPerHost = 10`), made the pool an environment variable, and ran the same Locust session as Eason (create, add, get, then 100 to 500 ms wait, 100 users, spawn 10/s, 4 workers, 5 minutes) with a pool of 200 and then 10. Only that variable changed between the runs, and both tasks logged the new value before the second run.

| 100 users, 5 minutes | Pool 200 | Pool 10 (SDK default) | Eason, pool 10 |
|---|---|---|---|
| Requests, failures | 148,639, 0 | 147,919, 0 | 192,750, 0 |
| Requests/s | 495.8 | 497.9 | 642.5 |
| Locust p50, p95, p99 | 98, 110, 120 ms | 97, 110, 120 ms | 39, 170, 290 ms |
| p95 minus p50 | 12 ms | 13 ms | 131 ms |
| In the service p50 / p95: get, add, create | 2.0 / 3.1, 3.7 / 12.1, 3.3 / 11.7 ms | 1.8 / 3.0, 3.6 / 12.0, 3.2 / 11.7 ms | 1.9 to 3.7, 90 to 97 ms |
| Inside DynamoDB avg: Get, Put, Update | 1.08, 2.38, 2.81 ms | 1.15, 2.45, 2.88 ms | 1.3 to 3.9 ms |
| ECS CPU average, max | 31%, 38% | 29%, 41% | 63 to 70%, 100% |

{figure:pools|charts/ddb-locust-pools.png|Locust percentiles: pool 10 and pool 200 are the same for me, and neither has the tail Eason saw}

The pool size made no difference. Both of my runs had a small tail, 12 to 13 ms between p50 and p95, and both medians are my 86 ms network plus a few ms. What differs from Eason's run is CPU: my tasks peaked at 41%, while Eason's hit 100%. At 100% CPU a 0.25 vCPU task queues requests, which fits Eason's in-service p95 of 90 to 97 ms. So I think that tail came from CPU saturation, and the SDK pool only matters once the tasks are that busy. Eason's tasks also handled about 30% more requests per second than mine, because Eason's laptop is closer to Oregon (each Locust user sends its next request sooner), and mine run on ARM64. With 2 tasks each, that's about 321 requests/s per task vs my 248. I didn't run on x86 or past 500 requests/s, so I can't separate those two effects.

One thing I noticed in the logs: for POSTs the time in the service at p95 (12 ms) is higher than the time inside DynamoDB (5 ms), but for GETs they're equal (3 ms). The extra time is before the DynamoDB call, so it's most likely the handler waiting for the request body from the ALB. It doesn't show at the median.

#### Evidence of the partition strategy working under load

On-demand absorbed about 20,150 write units and 5,040 read units a minute, the expected 4 to 1, with 0 read or write throttle events in either run ({fig:capacity}, {fig:errors}). Contributor Insights saw about 20,000 different cart IDs in each 5-minute run, about 4,700 to 5,400 a minute, and the busiest single key in any minute had 34 to 38 accesses ({fig:insights}). For comparison, the update probe below hammers one cart on purpose, and that key reached 7,005 a minute. So random 31-bit IDs spread the shopping load with no hot partition. The 2 conditional check failures in the graphs (1 per run) were cart ID collisions: the logs have 0 404s and 0 5xx for those runs, so they were creates that retried with a new ID. That's 2 in about 100,000 creates, close to Eason's 1 in 64,000.

{figures:ddblat|screenshots/step2-ddb-latency.png|DynamoDB SuccessfulRequestLatency, average and p99 per operation. GetItem about 1.1 ms, UpdateItem about 2.8 ms under load|capacity|screenshots/step2-ddb-capacity.png|Consumed capacity: about 20,150 write and 5,040 read units a minute in each Locust run}
{figures:errors|screenshots/step2-ddb-errors.png|0 throttle events and 0 system errors. The 2 conditional check failures are cart ID collisions that retried|insights|screenshots/step2-insights.png|Contributor Insights: about 5,000 cart IDs a minute under load, while the busiest single key spikes only during the hot-key probe at 08:56}
{figures:ecs|screenshots/step2-ecs.png|ECS CPU: about 30% average and at most 41% in both Locust runs, memory about 1.6%|alb|screenshots/step2-alb.png|ALB TargetResponseTime: about 4 ms average and 9 to 11 ms p95, about 30,000 requests a minute}

#### Eventual consistency behavior

`tests/consistency_test.py` runs the three scenarios from the assignment through the ALB. When a read is stale it reads again until the write shows up. Through the ALB, every read reaches DynamoDB about 86 ms after the write was acknowledged, so I also added an in-region probe (`POST /debug/consistency-probe`). It runs inside a task, writes, and then issues an eventually consistent GetItem microseconds after the write returns. That's the case Eason predicted could see a stale read.

| Scenario (`results/dynamodb_consistency_results.json`, `probe-*.json`) | Result |
|---|---|
| Create a cart, then get it right away, 100 times (through the ALB) | 0 stale reads |
| Add a product, then get the cart right away, 100 times on one cart (through the ALB) | 0 stale reads |
| 10 clients x 20 adds to the same cart at once, while 4 readers poll | 200 of 200 succeeded, quantity 200, version 201, 0 lost updates, 0 times a reader saw the version go backwards in 91 reads |
| In-region probe: PutItem, then GetItem right away, 5,000 times | **12 stale (0.24%)**: the cart didn't exist yet. Fresh again after a median of 5.8 ms, at most 22.5 ms |
| In-region probe: UpdateItem, then GetItem right away, 5,000 times on one cart | **5 stale (0.10%)**: the old version. Fresh again after a median of 4.4 ms, at most 10.8 ms |

Across the three probe batches the stale counts were 7, 0 and 5 for creates and 1, 4 and 0 for updates, so they come in bursts rather than at a steady rate. Every stale read cleared up within 22.5 ms, and none of my 200 checks through the ALB was stale.

### 4. Implementation notes

*Partition key strategy:* every access pattern is by cart ID, so `cart_id` is the partition key. DynamoDB has no auto increment, and a counter item would make every create write the same key (a hot partition). So the ID is random in 1 to 2^31-1, which keeps the HW5 spec's int32, with `attribute_not_exists` and a retry for collisions. Contributor Insights confirmed it: about 20,000 keys per run, and the busiest normal key had at most 38 accesses a minute.

*Key differences from MySQL:* there are no joins or transactions. The update expression does what the row lock and upsert do in MySQL, and the condition does what the foreign key does (a missing cart is a 404). The attribute format means numbers travel as strings with type `N`, map keys must be strings (product 7 is `"7"`), and `items` is a reserved word, so it's `#items`. I added a `version` attribute, incremented on every add, so the consistency tests can detect an old copy of a cart.

*Eventual consistency observations:* eventually consistent reads cost half as much and were stale only when the read followed the write within microseconds, 0.1 to 0.24% of the time and for at most 22.5 ms. A user who adds an item and then loads the cart is always at least one network round trip behind the write, so they won't see it. The real risk is server code that writes and then immediately reads. For that, I'd return the new item from the write (`ReturnValues = ALL_NEW`) or use `ConsistentRead`, and I'd use a strong read at checkout.

*What surprised me / what failed:* my first personal AWS account was a member of an AWS Organization whose policy denied DynamoDB, ECS, ECR and ELB, and `AdministratorAccess` doesn't override that. I had to use a standalone account. I expected to see stale reads easily, and through the ALB I never did. I also expected the SDK pool to explain Eason's tail, and it didn't at my load.

*NoSQL vs SQL trade-offs:* DynamoDB needed no instance, pool or schema, handled about 340 writes/s on-demand with 0 throttles, and my whole STEP II session was 209,514 write and 54,892 read units, about 14 cents on-demand. In exchange every access pattern had to be designed into the keys first: customer history needed a GSI before it could be queried, while MySQL just needed an index.

## Team comparison

Tian's report covers STEP I only, so this compares Eason, Shikha and Utkarsh.

#### Design

| | Eason | Shikha | Utkarsh |
|---|---|---|---|
| Table, billing | shopping-carts, on-demand | CS6650L2-dynamodb-carts, PAY_PER_REQUEST | hw8-shopping-carts, on-demand |
| Keys | partition key shopping_cart_id, no sort key | partition key cart_id, no sort key | partition key cart_id, no sort key |
| Items | embedded map, product ID to quantity | embedded map, product ID to quantity | embedded map, product ID to quantity, max 50 products |
| Cart ID | random, 1 to the largest int32 | random positive integer | random, 1 to the largest int32 |
| Create | PutItem with attribute_not_exists | PutItem | PutItem with attribute_not_exists, retry on collision |
| Get | GetItem, eventually consistent | GetItem, ConsistentRead=false | GetItem, eventually consistent, `?consistent=true` for strong |
| Add | UpdateItem with if_not_exists, and attribute_exists for a 404 | UpdateItem with if_not_exists | UpdateItem with if_not_exists, attribute_exists for a 404 and size(items) < 50 for a 409 |
| Other attributes | customer_id | customer_id, status, created_at, updated_at | customer_id, version, created_at, updated_at, expires_at (TTL) |
| Other setup | Contributor Insights, gateway VPC endpoint | not described | GSI on (customer_id, created_at), TTL, Contributor Insights, gateway VPC endpoint, ARM64 tasks |

#### Results

| | Eason | Shikha | Utkarsh |
|---|---|---|---|
| 150 test method | 50 sessions of create, add, get, one at a time | 3 phases, 50 creates, 50 adds, 50 gets, 25 at once | 3 phases, one at a time |
| 150 test success | 150 of 150, in 5.8 s | 150 of 150 | 150 of 150, in 13.8 s |
| Avg, P50 | 38.79, 38.77 ms | 112.93, 95.33 ms | 91.79, 89.70 ms |
| P95, P99 | 47.20, 51.39 ms | 203.53, 301.93 ms | 99.49, 130.56 ms |
| Avg create / add / get | 39.25 / 39.80 / 37.32 ms | 146.36 / 97.44 / 94.99 ms | 95.07 / 90.77 / 89.54 ms |
| Network only | about 35 ms | not measured | 85.9 ms (`/health`) |
| Inside AWS create / add / get | in the service: 4.32 / 3.88 / 2.12 ms | CloudWatch: PutItem 6.14 / UpdateItem 3.41 / GetItem 1.12 ms | in the service: 4.67 / 3.90 / 3.03 ms, CloudWatch: 3.19 / 2.93 / 1.23 ms |
| Consumed capacity | 128,957 write and 32,636 read units for the load test | 100 write and 25 read units for the 150 test | 100 write and 25 read units for the 150 test, about 20,150 and 5,040 a minute under load |
| Throttles | 0 | none observed | 0 |
| Stale reads | 0 of 200 | none observed | 0 of 200 through the ALB, 17 of 10,000 in-region (0.17%), all fresh within 22.5 ms |
| Same cart concurrent adds | 100 of 100, quantity 100 | 20 of 20, 3 to 23 | 200 of 200, quantity 200 |
| Load test | 100 users, 5 min, 642.5 req/s, p95 170 ms | not run | 100 users, 5 min, 496 to 498 req/s, p95 110 ms with pool 10 and pool 200 |
| ECS CPU under load | 63 to 70% average, 100% max | not run | about 30% average, 41% max |

**Where we match:**

- We came up with the same design: one item per cart with the items embedded as a map, the cart ID as the partition key with no sort key, on-demand billing, and random integer IDs instead of a counter. We also used the same operations, PutItem, an eventually consistent GetItem, and an UpdateItem with if_not_exists that adds to the quantity atomically.
- Inside AWS, create was the slowest DynamoDB call for all three of us, then add, then get: 4.32, 3.88 and 2.12 ms in Eason's service, 6.14, 3.41 and 1.12 ms in Shikha's CloudWatch, and 3.19, 2.93 and 1.23 ms in Utkarsh's CloudWatch.
- Writes vs reads were 4 to 1 for all of us: 100 vs 25 for both Shikha and Utkarsh in the 150 test, 128,957 vs 32,636 (3.95) for Eason, and about 20,150 vs 5,040 a minute for Utkarsh under load. Each session is 2 writes and 1 read, and an eventually consistent read is half a unit, so 2 to 0.5 is 4 to 1.
- 0 throttles and 0 lost updates for all three of us, and 0 stale reads through the API for all three of us. We all say eventual consistency only allows stale reads.
- DynamoDB itself is a small part of the client time. Shikha's calls took 1.1 to 6.1 ms and the ALB about 20 ms vs 112.93 ms from the client. Eason's took 2.1 to 4.3 ms in the service vs about 39 ms from the laptop. Utkarsh's took 1.2 to 3.2 ms vs 91.79 ms, of which 85.9 ms is the network.
- The partition key spread the load evenly for both of us who measured it: Eason saw about 39,000 cart IDs with no other key over about 55 units a minute, and Utkarsh saw about 20,000 per run with the busiest normal key at 34 to 38 a minute.

**Where we differ and why:**

- *Stale reads:* only Utkarsh saw any, and only with the in-region probe that reads microseconds after the write: 12 of 5,000 creates (0.24%) and 5 of 5,000 updates (0.10%), all fresh within 22.5 ms. Through the API none of us saw one, because every read reaches DynamoDB at least one network round trip after the write. That's exactly what Eason predicted: "a server that writes and reads in the same request could see it."
- *Load tail:* Eason's 100 user run had p95 170 ms, while Utkarsh's had p95 110 ms on an 86 ms network, only 12 ms above its median. Eason thought it was the SDK's 10 idle connections, and Utkarsh tested it: pool 10 and pool 200 gave the same p95 (110 ms) and p99 (120 ms). The difference is CPU. Eason's 0.25 vCPU tasks hit 100%, while Utkarsh's peaked at 41% at about 23% fewer requests per task (248 vs 321 req/s) and on ARM64. So the tail most likely came from CPU saturation, not the pool. Shikha's ECS CPU was about 0.21% during the 150 operation test, too light to show either effect.
- *Absolute latency:* 38.79 ms for Eason, 91.79 ms for Utkarsh and 112.93 ms for Shikha, but inside AWS all three are 1 to 6 ms. Eason's and Utkarsh's tests were both one at a time, so the difference is the network from our laptops (about 35 vs 86 ms). Shikha's test also sent 25 requests at once.
- *Partition evidence:* Eason's and Utkarsh's are from Contributor Insights under load, and Shikha's is 0 key-range throttle events at 150 operations.
- *Checking which backend answers:* Shikha's first DynamoDB benchmark was really served by an old image still using MySQL, and the sequential IDs, the empty table and the empty DynamoDB metrics gave it away. Eason hit the same risk from the other side, a test right after a switch could still reach the old tasks, and fixed it with wait_for_steady_state, which Utkarsh also used. Utkarsh's `/health` also returns the backend name, and both tasks log their settings at startup. Since all of us use random IDs for DynamoDB, the cart ID in the response shows which backend answered.
- *Create condition:* Eason's and Utkarsh's PutItem have attribute_not_exists, which caught 1 collision in about 64,000 carts for Eason and 2 in about 100,000 for Utkarsh. Shikha's report doesn't mention a condition on the create, and with 50 creates a collision is very unlikely.
- *Extra design:* only Utkarsh added a GSI for customer history, a TTL for abandoned carts and the 50-item limit in the condition. Eason skipped the GSI since none of the 3 endpoints needs history.
- *What failed:* Shikha's product lookup used int against int32 keys, Eason had to handle the attribute format, string map keys and items being a reserved word, and Utkarsh's first AWS account was blocked by an organization policy that denied DynamoDB and ECS.

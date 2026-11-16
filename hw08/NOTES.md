# HW8 — notes for Utkarsh's STEP II (DynamoDB) addition to the team report

Source: `~/Downloads/hw8-team-report 1.pdf` (26 pages). Team: Eason Liu, Tian, Shikha Sen (+ Utkarsh Sharma, STEP II).

## Report structure (must match)
- p1 "Who did what" table: rows STEP I / II / III, one column per teammate -> add Utkarsh column + name in Team line.
- Each person's section: 1. Terraform / table design, 2. API implementation, 3. Performance testing results, 4. Implementation notes.
- After each step: "Team comparison" -> Design table, Results table, "Where we match:", "Where we differ and why:".
- Figures numbered globally: Eason uses 1-7 (I), Tian 8-14 (I), Eason 15-19 (II), Eason 20-22 (III). My figures continue from 23.
- Writing style: first person, plain, numbers everywhere, short sentences, "about X ms".
- STEP II currently: Eason p12-14, Shikha p14-16, Team comparison p16-17 (Eason vs Shikha only). Tian: STEP I only.

## Assignment STEP II checklist (all must be covered)
- [ ] Part 1 table design: partition key strategy, sort key need, single vs multi table, embedded vs separate items, indexes
- [ ] Terraform: DynamoDB table added (MySQL is teammates' infra; mine is DynamoDB-only stack)
- [ ] Part 2 API: same 3 endpoints, SDK error handling, attribute value format, partition key in all ops, DynamoDB exceptions
- [ ] Part 3 consistency: create->get, add->get, rapid concurrent updates from multiple clients; how often, how fast, UX impact
- [ ] Part 4: identical 150-op test (50 create, 50 add, 50 get) -> dynamodb_test_results.json, same JSON format
- [ ] Part 5 learning notes: surprises, partition key first try / changes, hot partitions, validation
- [ ] Part 6 CloudWatch: request latency, consumed capacity, throttling, partition distribution (Contributor Insights), errors
- [ ] Deliverables: direct comparison with STEP I MySQL, partition evidence under load, consistency documentation, 1-page notes
- [ ] terraform destroy + verify nothing left

## Teammates' STEP II facts (for the 3-way comparison)
| | Eason | Shikha |
|---|---|---|
| Table | shopping-carts, on-demand, PK shopping_cart_id N, no SK | CS6650L2-dynamodb-carts, PAY_PER_REQUEST, PK cart_id, no SK |
| Items | map product->qty | map product->qty |
| IDs | random 1..maxint32, PutItem attribute_not_exists | random positive int, PutItem (no condition) |
| Get | GetItem eventual | GetItem ConsistentRead=false |
| Add | UpdateItem if_not_exists + attribute_exists (404) | UpdateItem if_not_exists |
| Extras | Contributor Insights, gateway VPC endpoint, no GSI | status/created_at/updated_at attrs |
| 150 test | 50 sessions create,add,get sequential, 5.8 s | 3 phases, 25 workers |
| Avg/P50 | 38.79 / 38.77 | 112.93 / 95.33 |
| P95/P99 | 47.20 / 51.39 | 203.53 / 301.93 |
| create/add/get avg | 39.25 / 39.80 / 37.32 | 146.36 / 97.44 / 94.99 |
| inside AWS | service 4.32 / 3.88 / 2.12 ms | CW PutItem 6.14, UpdateItem 3.41, GetItem 1.12 |
| capacity | 128,957 W / 32,636 R (load test) | 100 W / 25 R (150 test) |
| stale reads | 0 of 200 | none |
| same-cart concurrent | 10 clients x10, qty 100 | 20 concurrent, 3 -> 23 |
| load test | 100 users 5 min: 192,750 req, 642.5 rps, p50 39 p95 170 p99 290; ECS CPU 63-70% avg, 100% max; DynamoDB itself 1.3-3.9 ms | not run |
| failures | attribute format, string map keys, `items` reserved word | old MySQL image benchmarked; int vs int32 product key |

Open question left by Eason: p95 170 ms under load "I think it's the SDK opening new HTTPS connections ... only keeps 10 idle
connections per host by default, but I didn't test that." -> Utkarsh tests MaxIdleConnsPerHost 10 (SDK default) vs 200.

STEP I MySQL numbers (for comparison only, from team report):
- Tian (sequential 3-phase, closest to my method): avg 17.8, p50 16.9, p95 24.7, p99 28.1; create/add/get 17.2/20.9/15.2; in service 2.7/6.9/1.5
- Eason: avg 39.20, p95 47.00; create/add/get 38.98/40.40/38.22; in service 3.05/5.51/1.65; 100 users p95 50 ms, 698 rps
- Shikha: avg 112.56 (25 concurrent)

## My setup (differences to call out)
- Personal AWS account (Learner Lab budget exhausted), us-west-2, profile hw8.
- Fargate ARM64 (Graviton) 256 CPU / 512 MB, 2 tasks (same count as teammates), ALB, public subnets, no NAT, DynamoDB gateway endpoint.
- Table hw8-shopping-carts: PK cart_id N, GSI customer_id-index (customer_id, created_at) for customer history, TTL expires_at,
  Contributor Insights, version attribute for staleness detection, 50-distinct-product limit enforced in the condition.
- Tests: tests/load_test.py (3 phases sequential), tests/consistency_test.py (client + in-region probe), locust/ 100-user run.

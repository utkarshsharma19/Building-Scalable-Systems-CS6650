## Utkarsh Sharma's results

I ran all my STEP I tests in my own AWS account (us-west-2), after the STEP II runs, with the same 2 Fargate tasks (256 CPU, 512 MB, ARM64) behind an ALB. Code, results and screenshots are in `hw08/` in my repo.

### 1. Terraform: RDS MySQL

`terraform/rds.tf` adds RDS to the same Terraform as my DynamoDB table:

- MySQL 8.0 (AWS picked 8.0.46) on db.t3.micro, 20 GB gp2, single AZ.
- Two private subnets with no route to the internet. The security group only allows 3306 from the ECS tasks' security group, and `publicly_accessible = false`.
- `skip_final_snapshot = true`, `deletion_protection = false`, `backup_retention_period = 0`, and the password is a Terraform `random_password`.
- `terraform apply -var database=mysql` only replaces the task definition, so the same service switches databases. I checked `/health` (it returns the backend name) and the startup logs of both tasks before testing.

Both tasks logged `max_connections 61` when they connected, the same as Eason's. At the end, `terraform destroy` removed all 32 resources.

### 2. Shopping cart API and database schema

The handlers are the same for both databases (`main.go`), and `mysql.go` has the SQL. The service creates the tables at startup, since RDS is private:

```
shopping_carts (cart_id INT UNSIGNED AUTO_INCREMENT PK, customer_id, version, created_at, updated_at,
                INDEX idx_customer_created (customer_id, created_at))
cart_items     (cart_id, product_id, quantity CHECK (quantity > 0), added_at,
                PRIMARY KEY (cart_id, product_id),
                FOREIGN KEY (cart_id) REFERENCES shopping_carts ON DELETE CASCADE)
```

- *Two tables:* a cart has 0 to 50 items, so each item is its own row, and adding an item doesn't rewrite the whole cart.
- *Keys:* `(cart_id, product_id)` means a product is in a cart only once, and adding it again adds to the quantity. InnoDB stores rows in primary key order, so all items of a cart are next to each other. The foreign key stops orphaned items, and `ON DELETE CASCADE` removes items with their cart.
- *Indexes:* cart lookups use the primary keys, so the only extra index is `(customer_id, created_at)` for customer history.
- *EXPLAIN* (`results/mysql_explain.txt`): GET is `const` on `shopping_carts` plus `ref` on the `cart_items` primary key. Customer history uses `idx_customer_created` with a backward index scan and no sort. When I tried `ORDER BY product_id` on the GET, EXPLAIN showed `Using temporary; Using filesort`, the same thing Eason found, so I sort the items in Go.

| Endpoint | SQL | Responses |
|---|---|---|
| POST /shopping-carts | 1 INSERT | 201, 400 |
| GET /shopping-carts/{id} | 1 SELECT ... LEFT JOIN cart_items | 200, 400, 404 |
| POST /shopping-carts/{id}/items | transaction (below) | 204, 400, 404, 409 for a 51st product |

Add item transaction: (1) `SELECT` the item count and whether this product is already there `FROM shopping_carts WHERE cart_id = ? FOR UPDATE`, 404 if there's no row, 409 if it would be the 51st product. (2) `INSERT INTO cart_items ... AS new ON DUPLICATE KEY UPDATE quantity = cart_items.quantity + new.quantity`. (3) `UPDATE shopping_carts SET version = version + 1, updated_at = ...`. (4) Commit. A deadlock or lock timeout (MySQL errors 1213 and 1205) is retried up to 2 times. Every query uses `?` placeholders, and the handler checks all IDs and the quantity before anything reaches the database. Database errors are logged and return a 500, so "not found" and "database broke" are different.

Connection pool (`database/sql`): `MaxOpenConns = 10` per task, `MaxIdleConns = 10` so connections are kept between bursts, `ConnMaxLifetime = 5 min` and `ConnMaxIdleTime = 2 min`. With 2 tasks that's 20 of RDS's 61 connections. The service logs `db.Stats()` every 10 s, which Tian recommended.

### 3. Performance testing results

#### The 150 operation test

`tests/load_test.py` sends 50 creates, then one add to each of those carts, then one get of each, one at a time from my laptop, the same test I ran on DynamoDB. It took 13.3 s and all 150 succeeded (`results/mysql_test_results.json`).

| Operation | n | Success | Avg | P50 | P95 | P99 | In the service, avg | Inside MySQL, avg |
|---|---|---|---|---|---|---|---|---|
| create_cart | 50 | 50 | 88.11 ms | 85.23 ms | 96.19 ms | 143.48 ms | 3.09 ms | 2.84 ms |
| add_items | 50 | 50 | 92.51 ms | 90.35 ms | 102.53 ms | 130.65 ms | 9.24 ms | 9.02 ms |
| get_cart | 50 | 50 | 85.55 ms | 85.14 ms | 90.15 ms | 97.31 ms | 2.20 ms | 2.17 ms |
| all | 150 | 100% | 88.72 ms | 86.03 ms | 97.69 ms | 130.65 ms | | |

As in STEP II, about 86 ms of every request is the network from my laptop to Oregon (`/health` averaged 85.9 ms). Get cart is about 2 ms in the service, well under the 50 ms goal. Add item is the slowest because it's 3 statements and a commit, and the commit writes to disk.

#### 100 users for 5 minutes

I ran the same Locust session as on DynamoDB: create, add, get, then a 100 to 500 ms wait, with 100 users.

| | MySQL |
|---|---|
| Requests, failures | 147,470, 0 |
| Requests/s | 492.3 |
| Locust p50, p95, p99 | 98, 110, 160 ms (add item p99 170 ms) |
| In the service p50 / p95 / p99 | get 1.5 / 2.8 / 17.6 ms, create 2.7 / 11.3 / 34.1 ms, add 7.4 / 17.4 / 66.1 ms |
| RDS CPU | 24 to 30% |
| RDS connections | 20 (2 tasks x 10) |
| RDS IOPS | about 446 writes/s, about 0.4 reads/s |
| RDS write latency | about 0.5 ms |
| ECS CPU average, max | 17%, 21% |

#### Connection pool behavior

Both tasks opened all 10 connections and kept them. The 10 s snapshots never caught more than 4 in use on one task (averages 1.4 and 0.5). Even so, 1,522 requests (1.1% of about 144,000) had to wait for a free connection, 61.5 s in total, about 40 ms each. So, like Eason and Tian, I found the pool is mostly idle and the waits come in short bursts. Those waits are why MySQL's p99 (160 ms) is higher than DynamoDB's (120 ms) in the same test, while the p50 and p95 are the same. A bigger pool would cut the waits, but tasks × pool size has to stay under the 61 connections.

RDS CPU was 24 to 30% with 20 connections, so the database wasn't the limit. Reads were almost 0 IOPS because the data fits in memory. Writes cost disk work: each session is 2 writes out of 3 requests, so 492 × 2/3 is about 328 commits a second, and RDS showed about 446 write IOPS.

{figures:rdscpu|screenshots/step1-rds-cpu-conn.png|RDS CPU and connections: 2 during the one-at-a-time tests, 10 in the 10-client concurrency test, 20 with 100 users|rdsiops|screenshots/step1-rds-iops.png|RDS IOPS: about 446 writes/s and almost 0 reads with 100 users}
{figures:rdslat|screenshots/step1-rds-latency.png|RDS read and write latency, about 0.5 ms for writes|mysqlecs|screenshots/step1-ecs.png|ECS CPU with MySQL: about 17% average, 21% max with 100 users}

#### Concurrency check

10 clients each added 20 items to the same cart at the same time while 4 readers kept reading it: 200 of 200 succeeded, the final quantity was 200 and the version was 201, so no updates were lost. 100 create-then-get and 100 add-then-get checks were all fresh, as expected with one primary.

### 4. Implementation notes

*Compared to in-memory (HW5):* my HW5 in-memory store took about 4 µs at p50 and 126 µs at p99 inside the service. MySQL takes about 2 ms for a get and 7 to 9 ms for an add, about 500 to 2,000 times slower. Next to the 86 ms network to my laptop, though, a user barely notices it. In exchange the carts survive restarts, and both tasks see the same carts. With an in-memory map each task has its own data, so a cart created on one task would 404 on the other, which Eason measured (66.67% success).

*What didn't work at first:* the MySQL code itself worked the first time on AWS, because I tested it against a local MySQL 8.0 in Docker first (16 of 16 functional checks, the 150 test and the concurrency test). What I got wrong was the measurement: my first query for the in-service times used a time window that also caught the start of the consistency test, so it counted 219 gets instead of 50. I narrowed the window to the exact test. Two things I planned for ahead of time: RDS can still be starting when a task boots, so the service retries the connection for up to 2 minutes, and MySQL 8.0.20+ deprecates `VALUES()` in `ON DUPLICATE KEY UPDATE`, so I used the `AS new` row alias.

*Did the first schema meet the requirements?* Yes. Every query uses a primary key or the customer index, and get cart was about 2 ms. The one change was moving the item sort out of SQL.

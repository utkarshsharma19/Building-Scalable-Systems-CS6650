#### Adding Utkarsh's MySQL results

Utkarsh's STEP I section came after the comparison above was written, so this adds Utkarsh's results to it.

| | Eason | Tian | Shikha | Utkarsh |
|---|---|---|---|---|
| Tables, cart_items key | shopping_carts, cart_items, (shopping_cart_id, product_id) | products, carts, cart_items, (cart_id, product_id) | shopping_carts, cart_items, (cart_id, product_id) | shopping_carts, cart_items, (cart_id, product_id) |
| Add item | FOR UPDATE, upsert | FOR UPDATE, upsert, update updated_at | FOR UPDATE, upsert | FOR UPDATE (also checks the 50-item limit), upsert, bump version, deadlock retry |
| Pool per task | 10 | 20 (also tested 5) | 25 | 10 |
| 150 test order | 50 sessions, one at a time | 3 phases, one at a time | 3 phases, 25 at once | 3 phases, one at a time |
| Avg, P50 | 39.20, 38.94 ms | 17.8, 16.9 ms | 112.56, 99.82 ms | 88.72, 86.03 ms |
| P95, P99 | 47.00, 50.20 ms | 24.7, 28.1 ms | 184.42, 278.77 ms | 97.69, 130.65 ms |
| In the service create / add / get | 3.05 / 5.51 / 1.65 ms | 2.7 / 6.9 / 1.5 ms | not measured | 3.09 / 9.24 / 2.20 ms |
| Network only | about 35 ms | about 13 to 14 ms | not measured | 85.9 ms |
| 100 users: req/s, fails | about 698, 0 | 300 to 307, 0 | not run | 492, 0 |
| 100 users: p50, p95, p99 | 38, 50, 96 ms | 13 to 16, 22 to 31, 59 to 130 ms | not run | 98, 110, 160 ms |
| RDS CPU, connections | 24 to 30%, 18 to 21 | 18 to 24%, 40 (pool 20) | about 5.79%, about 17 | 24 to 30%, 20 |
| Pool waits under load | 1.9% of requests, about 36 ms each | 51 waits (pool 20), 351 (pool 5) | not logged | 1.1% of requests, about 40 ms each |
| Same-cart concurrency | 10 clients, quantity 100 | not run | 40 updates, quantity 40 | 10 clients x 20, quantity 200 |

- *Where Utkarsh matches:* the same two-table design and FOR UPDATE + upsert as everyone, and the same in-service times as Eason and Tian (get about 2 ms, add the slowest because of the commit). Utkarsh's pool of 10 behaved almost exactly like Eason's: all 10 open, at most 4 in use, and about 1 to 2% of requests waiting about 40 ms in bursts. Utkarsh's RDS CPU (24 to 30%) at about 492 req/s was the same as Eason's at about 698.
- *Where Utkarsh differs and why:* the client times are higher (88.72 ms average) only because Utkarsh's laptop is about 86 ms from Oregon. The test was one request at a time, like Tian's, but over a longer network. Utkarsh's add item in the service (9.24 ms) is a bit slower than Eason's and Tian's (5.5 to 6.9 ms) because the transaction also counts the cart's items for the 50-item limit.

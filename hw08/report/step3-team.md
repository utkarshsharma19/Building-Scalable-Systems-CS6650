## Team comparison: adding Utkarsh's results

Utkarsh joined the report after the comparison and summary above were written, so this page adds Utkarsh's STEP III results to them.

#### Part 0 and Part 1

Utkarsh's `mysql_test_results.json` and `dynamodb_test_results.json` both have 150 operations, 50 of each, all successful, and `combined_results.json` has 300 records ({fig:verify}).

| Metric | Eason MySQL / DynamoDB | Shikha MySQL / DynamoDB | Tian MySQL | Utkarsh MySQL / DynamoDB | Utkarsh's winner, margin |
|---|---|---|---|---|---|
| Avg (ms) | 39.20 / 38.79 | 112.56 / 112.93 | 17.8 | 88.72 / 91.79 | MySQL, 3.07 |
| P50 (ms) | 38.94 / 38.77 | 99.82 / 95.33 | 16.9 | 86.03 / 89.70 | MySQL, 3.67 |
| P95 (ms) | 47.00 / 47.20 | 184.42 / 203.53 | 24.7 | 97.69 / 99.49 | MySQL, 1.80 |
| P99 (ms) | 50.20 / 51.39 | 278.77 / 301.93 | 28.1 | 130.65 / 130.56 | DynamoDB, 0.09 |
| Success (%) | 100 / 100 | 100 / 100 | 100 | 100 / 100 | Tie |
| CREATE_CART avg | 38.98 / 39.25 | 138.28 / 146.36 | 17.2 | 88.11 / 95.07 | MySQL, 6.96 |
| ADD_ITEMS avg | 40.40 / 39.80 | 104.60 / 97.44 | 20.9 | 92.51 / 90.77 | DynamoDB, 1.74 |
| GET_CART avg | 38.22 / 37.32 | 94.81 / 94.99 | 15.2 | 85.55 / 89.54 | MySQL, 3.99 |

- *Where Utkarsh matches:* all three of us who ran both databases found MySQL faster at CREATE and DynamoDB faster at ADD, and all three called the 150 test close to a tie (margins of 0.41, 0.37 and 3.07 ms). Utkarsh's in-service times explain it the same way Eason's did: create 3.09 vs 4.67 ms and add 9.24 vs 3.90 ms.
- *Where Utkarsh differs:* MySQL won Utkarsh's average and GET, as for Shikha, while they went to DynamoDB for Eason. All of those margins are under 4 ms on top of an 86 ms network, so they're still noise.

#### Consistency, resources and scenarios

| | Eason | Shikha | Utkarsh |
|---|---|---|---|
| DynamoDB stale reads through the API | 0 of 200 | none | 0 of 200 |
| DynamoDB stale reads in-region | not tested | not tested | 17 of 10,000 (0.17%), all fresh within 22.5 ms |
| Concurrent adds to one cart | 100 of 100 on both | 20 of 20 (DynamoDB), 40 of 40 (MySQL) | 200 of 200 on both |
| 100 users, p95 / p99, MySQL vs DynamoDB | 50 / 96 vs 170 / 290 ms | not run | 110 / 160 vs 110 / 120 ms |
| ECS CPU under load, MySQL vs DynamoDB | about 76% vs 63 to 70% (100% max) | not run | about 17% vs 30% |
| Scenario A / B / C / D | DynamoDB / MySQL / DynamoDB / DynamoDB + MySQL | MySQL / MySQL / DynamoDB / DynamoDB + polyglot | DynamoDB / MySQL / DynamoDB / DynamoDB + MySQL |
| Shopping cart winner | MySQL, for the traffic tested | DynamoDB | DynamoDB |
| Polyglot: sessions, catalog, order history | DynamoDB, MySQL, MySQL | DynamoDB, MySQL, MySQL | DynamoDB, MySQL, MySQL |

- Utkarsh's results agree with Eason's on scenarios A to D and with Shikha's on the cart winner. All three of us who wrote a polyglot plan put sessions in DynamoDB and the catalog and order history in MySQL.
- *Load results:* Eason's and Utkarsh's 100-user tests point in opposite directions. Eason saw a worse DynamoDB tail (p95 170 vs 50 ms), and Utkarsh saw a worse MySQL tail (p99 160 vs 120 ms). The difference is the tasks: Eason's DynamoDB tasks hit 100% CPU, and Utkarsh's never went over 41%. Utkarsh also tested Eason's SDK pool idea and found pool 10 and pool 200 gave the same results. So once the service has CPU headroom, DynamoDB's tail is the smaller one, and the MySQL pool waits (1.1 to 1.9% of requests, about 36 to 40 ms each, for both Eason and Utkarsh) become the tail.
- *Consistency:* only Utkarsh's in-region probe produced stale reads, which confirms what Eason and Shikha wrote: eventual consistency allows stale reads, but you only see them when the read follows the write within milliseconds.

#### Updated Team Summary

With Utkarsh's results added, our conclusion holds: inside AWS both databases take a few milliseconds, and our client numbers mostly measure the network from each of our laptops (about 14, 35 and 86 ms for Tian, Eason and Utkarsh) and how many requests we sent at once. Everyone who ran both databases found MySQL faster at creates and DynamoDB faster at adds. Every test succeeded, every concurrent update test ended at the exact quantity, and stale reads appeared only in Utkarsh's in-region probe, 0.17% of the time and for at most 22.5 ms. The open question from Eason's load test now has an answer: the SDK connection pool wasn't the cause, and the tail most likely came from the tasks running out of CPU. With enough CPU, Utkarsh's DynamoDB had the lower tail. That supports DynamoDB for carts (Shikha's and Utkarsh's pick), as long as the service tasks are big enough. DynamoDB's HTTPS calls used more service CPU than MySQL for Utkarsh (30% vs 17%), and Eason's MySQL pick shows what happens when the tasks are too small. We still agree on sessions in DynamoDB and the catalog and order history in MySQL.

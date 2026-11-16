<h1 class="title">CS6650 HW8 Team Report</h1>

Team: Eason Liu, Tian, Shikha Sen, Utkarsh Sharma

### Who did what

Each teammate's section is their own implementation, test data and notes, in their own words. The Team comparison after each step compares our results and explains the differences, and the Team Summary at the end is our overall conclusion.

| | Eason Liu | Tian | Shikha Sen | Utkarsh Sharma |
|---|---|---|---|---|
| STEP I: MySQL | RDS module, cart API, 150 operation test, 100 user Locust test, Week 5 in-memory comparison, CloudWatch | RDS module, cart API, 150 operation test, 100 user Locust tests with pool sizes 20 and 5, HW5 comparison, CloudWatch | RDS MySQL, cart API, 150 operation test, 40 update concurrency test, CloudWatch | RDS MySQL in the same Terraform, cart API with a shared handler layer, 150 operation test, 100 user Locust test, concurrency and EXPLAIN checks, CloudWatch |
| STEP II: DynamoDB | DynamoDB table, same API, 150 operation test, 100 user Locust test, consistency tests, CloudWatch and Contributor Insights | Tian's report covers STEP I only | DynamoDB table, same API, 150 operation test, consistency tests, CloudWatch | DynamoDB table with GSI and TTL, same API, 150 operation test, 100 user Locust tests with SDK pool 10 vs 200, consistency tests with an in-region probe, CloudWatch and Contributor Insights |
| STEP III: Comparison | Analysis from Eason's `combined_results.json` | Tian's MySQL results are part of the team comparison | Analysis from Shikha's `combined_results.json`, 2 to 3 page comparison report | Analysis from Utkarsh's `combined_results.json` |

Each of us has our own code, result files and screenshots in our own repo. Eason's, Tian's and Utkarsh's CloudWatch graphs are included below, and Shikha's screenshots are in Shikha's repo. Utkarsh's sections come after the other teammates' sections in each step, and Utkarsh's results are added to each Team comparison. Utkarsh's code and results are at `github.com/utkarshsharma19/Building-Scalable-Systems-CS6650` in `hw08/`.

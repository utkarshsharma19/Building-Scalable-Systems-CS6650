"""HW7 load tests. Pick the user class on the command line:

  locust -f locustfile.py SyncUser  ...   # POST /orders/sync
  locust -f locustfile.py AsyncUser ...   # POST /orders/async

Never point this at the system while the Lambda is subscribed to SNS.
"""
import random

from locust import HttpUser, between, task

PRODUCTS = [f"p{i:03d}" for i in range(1, 51)]


def order_body():
    return {
        "customer_id": random.randint(1, 10_000),
        "items": [
            {
                "product_id": random.choice(PRODUCTS),
                "quantity": random.randint(1, 3),
                "price": round(random.uniform(5, 200), 2),
            }
            for _ in range(random.randint(1, 4))
        ],
    }


class SyncUser(HttpUser):
    wait_time = between(0.1, 0.5)

    @task
    def sync_order(self):
        # 70 s client timeout: longer than the ALB's 60 s idle timeout, so a
        # request stuck in the payment line shows up as the ALB's 504.
        with self.client.post("/orders/sync", json=order_body(), name="POST /orders/sync",
                              timeout=70, catch_response=True) as r:
            if r.status_code != 200:
                r.failure(f"status {r.status_code}")


class AsyncUser(HttpUser):
    wait_time = between(0.1, 0.5)

    @task
    def async_order(self):
        with self.client.post("/orders/async", json=order_body(), name="POST /orders/async",
                              timeout=10, catch_response=True) as r:
            if r.status_code != 202:
                r.failure(f"status {r.status_code}")

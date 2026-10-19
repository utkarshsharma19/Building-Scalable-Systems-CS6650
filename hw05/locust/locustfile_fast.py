import random

from locust import FastHttpUser, between, task

SEEDED_PRODUCTS = 1000


def product_body(pid):
    return {
        "product_id": pid,
        "sku": f"SKU-{pid:05d}",
        "manufacturer": "Locust Load Co",
        "category_id": random.randint(1, 50),
        "weight": random.randint(0, 5000),
        "some_other_id": random.randint(1, 1000),
    }


class FastProductUser(FastHttpUser):
    wait_time = between(0, 0.1)

    @task(7)
    def get_product(self):
        pid = random.randint(1, SEEDED_PRODUCTS)
        self.client.get(f"/products/{pid}", name="GET /products/[id]")

    @task(2)
    def post_details(self):
        pid = random.randint(1, SEEDED_PRODUCTS)
        self.client.post(f"/products/{pid}/details", json=product_body(pid),
                         name="POST /products/[id]/details")

    @task(1)
    def get_missing(self):
        pid = random.randint(SEEDED_PRODUCTS + 1, 10_000_000)
        with self.client.get(f"/products/{pid}", name="GET /products/[missing]",
                             catch_response=True) as r:
            if r.status_code == 404:
                r.success()
            else:
                r.failure(f"expected 404, got {r.status_code}")

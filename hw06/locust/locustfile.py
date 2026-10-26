import random

from locust import FastHttpUser, between, task

TERMS = [
    "Electronics", "electro", "Books", "home", "Garden", "TOYS", "sports",
    "Alpha", "beta", "Gamma", "Product",
]


class SearchUser(FastHttpUser):
    wait_time = between(0, 0.01)

    @task
    def search(self):
        q = random.choice(TERMS)
        with self.client.get(f"/products/search?q={q}", name="GET /products/search?q=[term]",
                             catch_response=True) as r:
            if r.status_code != 200:
                r.failure(f"status {r.status_code}")
            elif r.headers.get("X-Products-Checked") != "100":
                r.failure(f"checked {r.headers.get('X-Products-Checked')} products, want 100")

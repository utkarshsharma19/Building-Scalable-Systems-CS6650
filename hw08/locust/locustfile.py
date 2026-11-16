"""100-user load test, same session as Eason's STEP II run so the numbers compare:
each user creates a cart, adds an item, gets the cart, then waits 100-500 ms."""
import random
from locust import HttpUser, task, between


class Shopper(HttpUser):
    wait_time = between(0.1, 0.5)

    @task
    def session(self):
        r = self.client.post("/shopping-carts", json={"customer_id": random.randint(1, 100000)}, name="create_cart")
        if r.status_code != 201:
            return
        cid = r.json()["shopping_cart_id"]
        self.client.post(f"/shopping-carts/{cid}/items",
                         json={"product_id": random.randint(1, 1000), "quantity": random.randint(1, 5)}, name="add_items")
        self.client.get(f"/shopping-carts/{cid}", name="get_cart")

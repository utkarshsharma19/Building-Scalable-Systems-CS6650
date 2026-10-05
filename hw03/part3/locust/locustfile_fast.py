import random
import uuid

from locust import FastHttpUser, task


class AlbumUser(FastHttpUser):
    @task(3)
    def get_album(self):
        self.client.get(f"/albums/{random.choice('123')}", name="GET /albums/[id]")

    @task(1)
    def post_album(self):
        self.client.post("/albums", name="POST /albums", json={
            "id": uuid.uuid4().hex, "title": "Load Test", "artist": "Locust", "price": 9.99,
        })

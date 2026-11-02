"""Locust load for Experiments 2 and 3.

FILE   which test file every user fetches (default f64k.bin = 1024 chunks).
SHAPE  "step" turns on the Experiment 3 step-load shape (ramp up, hold, drop to idle) so the
       auto scalers see a clear rise and fall. Without it, use -u/-r/-t on the command line.

Responses are grouped by status code in the Locust stats (name "/files [503]" etc.) so the
failure mix is visible directly in the UI and in the CSV.
"""
import os

from locust import HttpUser, LoadTestShape, constant, task

FILE = os.environ.get("FILE", "f64k.bin")


class FileUser(HttpUser):
    wait_time = constant(0.5)

    @task
    def fetch(self):
        with self.client.get("/files", params={"name": FILE}, name="/files",
                             catch_response=True, timeout=120) as r:
            if r.status_code == 200:
                r.success()
            else:
                r.failure(f"HTTP {r.status_code}")


if os.environ.get("SHAPE") == "step":
    class StepShape(LoadTestShape):
        # (end_second, users, spawn_rate)
        stages = [
            (120, 5, 5),     # 0-2 min: light load, baseline
            (720, 40, 2),    # 2-12 min: heavy load, should trigger scale-out
            (1680, 1, 5),    # 12-28 min: nearly idle, should trigger scale-in
        ]

        def tick(self):
            t = self.get_run_time()
            for end, users, rate in self.stages:
                if t < end:
                    return users, rate
            return None

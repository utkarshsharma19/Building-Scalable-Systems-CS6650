"""Load shapes for Part III. Pick one with SHAPE=ramp|spike|sustained|waves."""
import os

from locust import LoadTestShape

SHAPES = {
    "ramp": [(120 * (i + 1), 10 * (i + 1), 2) for i in range(8)] + [(1260, 80, 2)],
    "spike": [(180, 10, 10), (600, 80, 80), (1200, 10, 80)],
    "sustained": [(1200, 60, 60)],
    "waves": [(150 * (i + 1), 80 if i % 2 else 10, 80) for i in range(8)],
}


class StagedShape(LoadTestShape):
    stages = SHAPES[os.environ.get("SHAPE", "ramp")]

    def tick(self):
        t = self.get_run_time()
        for end, users, rate in self.stages:
            if t < end:
                return users, rate
        return None

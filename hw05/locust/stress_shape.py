from locust import LoadTestShape

STEPS = [50, 100, 250, 500, 1000, 1500, 2000]
STEP_SECONDS = 60
SPAWN_RATE = 50


class StepLoadShape(LoadTestShape):
    def tick(self):
        step = int(self.get_run_time() // STEP_SECONDS)
        if step >= len(STEPS):
            return None
        return STEPS[step], SPAWN_RATE

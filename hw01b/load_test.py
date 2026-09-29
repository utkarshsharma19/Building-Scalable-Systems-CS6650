import requests
import time
import matplotlib.pyplot as plt
import numpy as np

def load_test(url, duration_seconds=60):
    response_times = []
    start_time = time.time()
    end_time = start_time + duration_seconds
    print(f"Starting load test for {duration_seconds} seconds...")
    while time.time() < end_time:
        try:
            start_request = time.time()
            response = requests.get(url, timeout=10)
            end_request = time.time()
            response_time = (end_request - start_request) * 1000
            response_times.append(response_time)
            if response.status_code == 200:
                print(f"Request {len(response_times)}: {response_time:.2f}ms")
            else:
                print(f"Request {len(response_times)}: Failed with status {response.status_code}")
        except requests.exceptions.RequestException as e:
            print(f"Request failed: {e}")
    return response_times

EC2_URL = "http://16.145.225.51:8080/albums"
response_times = load_test(EC2_URL, duration_seconds=600)

plt.figure(figsize=(12, 8))
plt.subplot(2, 1, 1)
plt.hist(response_times, bins=50, alpha=0.7, color='blue')
plt.xlabel('Response Time (ms)')
plt.ylabel('Frequency')
plt.title('Distribution of Response Times')
plt.subplot(2, 1, 2)
plt.scatter(range(len(response_times)), response_times, alpha=0.6)
plt.xlabel('Request Number')
plt.ylabel('Response Time (ms)')
plt.title('Response Times Over Time')
plt.tight_layout()
plt.savefig("load_test_results.png", dpi=150)
plt.show()

print(f"\nStatistics:")
print(f"Total requests: {len(response_times)}")
print(f"Average response time: {np.mean(response_times):.2f}ms")
print(f"Median response time: {np.median(response_times):.2f}ms")
print(f"95th percentile: {np.percentile(response_times, 95):.2f}ms")
print(f"99th percentile: {np.percentile(response_times, 99):.2f}ms")
print(f"Max response time: {max(response_times):.2f}ms")

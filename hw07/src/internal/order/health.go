package order

import (
	"net/http"
	"os"
	"time"
)

// RunHealthcheckIfAsked lets a distroless image (no curl) serve as its own
// ECS container health check: `/app -healthcheck` exits 0 if GET /health is 200.
func RunHealthcheckIfAsked(port string) {
	if len(os.Args) < 2 || os.Args[1] != "-healthcheck" {
		return
	}
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + port + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	os.Exit(0)
}

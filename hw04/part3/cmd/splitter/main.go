package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"hw04mr/internal/s3util"
)

type response struct {
	Chunks     []string `json:"chunks"`
	ChunkBytes []int    `json:"chunk_bytes"`
	TotalMs    int64    `json:"total_ms"`
}

func isSpace(b byte) bool { return b == ' ' || b == '\n' || b == '\t' || b == '\r' }

// cut on whitespace so no word is split across chunks
func split(data []byte, n int) []int {
	cuts := []int{0}
	for i := 1; i < n; i++ {
		p := max(len(data)*i/n, cuts[len(cuts)-1])
		for p < len(data) && !isSpace(data[p]) {
			p++
		}
		cuts = append(cuts, p)
	}
	return append(cuts, len(data))
}

func main() {
	client, err := s3util.New()
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/split", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		url := r.URL.Query().Get("url")
		n, err := strconv.Atoi(r.URL.Query().Get("n"))
		if err != nil || n <= 0 {
			n = 3
		}
		bucket, _, err := s3util.Parse(url)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data, err := client.Get(r.Context(), url)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		job := strconv.FormatInt(start.UnixNano(), 36)
		cuts := split(data, n)
		var resp response
		for i := 0; i < n; i++ {
			chunk := data[cuts[i]:cuts[i+1]]
			out := s3util.URL(bucket, fmt.Sprintf("jobs/%s/chunks/chunk-%d.txt", job, i))
			if err := client.Put(r.Context(), out, chunk); err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			resp.Chunks = append(resp.Chunks, out)
			resp.ChunkBytes = append(resp.ChunkBytes, len(chunk))
		}
		resp.TotalMs = time.Since(start).Milliseconds()
		json.NewEncoder(w).Encode(resp)
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}

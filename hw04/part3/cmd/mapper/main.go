package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"hw04mr/internal/s3util"
	"hw04mr/internal/wc"
)

type response struct {
	Output     string `json:"output"`
	ChunkBytes int    `json:"chunk_bytes"`
	TotalWords int    `json:"total_words"`
	CountMs    int64  `json:"count_ms"`
	TotalMs    int64  `json:"total_ms"`
}

func main() {
	client, err := s3util.New()
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/map", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		url := r.URL.Query().Get("url")
		data, err := client.Get(r.Context(), url)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		countStart := time.Now()
		result := wc.Count(string(data))
		countMs := time.Since(countStart).Milliseconds()

		body, _ := json.Marshal(result)
		out := strings.TrimSuffix(strings.Replace(url, "/chunks/", "/mapped/", 1), ".txt") + ".json"
		if err := client.Put(r.Context(), out, body); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(response{
			Output:     out,
			ChunkBytes: len(data),
			TotalWords: result.TotalWords,
			CountMs:    countMs,
			TotalMs:    time.Since(start).Milliseconds(),
		})
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}

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
	Output      string `json:"output"`
	TotalWords  int    `json:"total_words"`
	UniqueWords int    `json:"unique_words"`
	TotalMs     int64  `json:"total_ms"`
}

func main() {
	client, err := s3util.New()
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/reduce", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		urls := strings.Split(r.URL.Query().Get("urls"), ",")
		prefix, _, ok := strings.Cut(urls[0], "/mapped/")
		if !ok {
			http.Error(w, "expected mapper output urls", http.StatusBadRequest)
			return
		}

		var final wc.Result
		for _, u := range urls {
			data, err := client.Get(r.Context(), u)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			var part wc.Result
			if err := json.Unmarshal(data, &part); err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			wc.Merge(&final, part)
		}

		body, _ := json.Marshal(final)
		out := prefix + "/final/counts.json"
		if err := client.Put(r.Context(), out, body); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(response{
			Output:      out,
			TotalWords:  final.TotalWords,
			UniqueWords: final.UniqueWords,
			TotalMs:     time.Since(start).Milliseconds(),
		})
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}

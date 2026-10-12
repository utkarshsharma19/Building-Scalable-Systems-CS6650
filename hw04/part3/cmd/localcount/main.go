package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"hw04mr/internal/wc"
)

func main() {
	in := flag.String("in", "", "input text file")
	out := flag.String("out", "", "output counts json")
	flag.Parse()

	data, err := os.ReadFile(*in)
	if err != nil {
		log.Fatal(err)
	}
	start := time.Now()
	result := wc.Count(string(data))
	ms := time.Since(start).Milliseconds()

	if *out != "" {
		body, _ := json.Marshal(result)
		if err := os.WriteFile(*out, body, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf(`{"bytes":%d,"total_words":%d,"unique_words":%d,"count_ms":%d}`+"\n",
		len(data), result.TotalWords, result.UniqueWords, ms)
}

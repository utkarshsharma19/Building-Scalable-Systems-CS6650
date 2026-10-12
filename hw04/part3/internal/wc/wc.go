package wc

import (
	"strings"
	"unicode"
)

type Result struct {
	TotalWords  int            `json:"total_words"`
	UniqueWords int            `json:"unique_words"`
	Counts      map[string]int `json:"counts"`
}

func isDelim(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\''
}

func Count(text string) Result {
	counts := make(map[string]int)
	total := 0
	add := func(w string) {
		w = strings.Trim(w, "'")
		if w != "" {
			counts[w]++
			total++
		}
	}
	lower := strings.ToLower(text)
	start := -1
	for i, r := range lower {
		switch {
		case isDelim(r) && start >= 0:
			add(lower[start:i])
			start = -1
		case !isDelim(r) && start < 0:
			start = i
		}
	}
	if start >= 0 {
		add(lower[start:])
	}
	return Result{TotalWords: total, UniqueWords: len(counts), Counts: counts}
}

func Merge(dst *Result, src Result) {
	if dst.Counts == nil {
		dst.Counts = make(map[string]int)
	}
	for w, c := range src.Counts {
		dst.Counts[w] += c
	}
	dst.TotalWords += src.TotalWords
	dst.UniqueWords = len(dst.Counts)
}

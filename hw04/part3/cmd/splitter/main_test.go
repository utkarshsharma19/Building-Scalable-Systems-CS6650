package main

import (
	"reflect"
	"strings"
	"testing"

	"hw04mr/internal/wc"
)

// Splitting then counting each chunk and merging must equal counting the whole text.
func TestSplitPreservesCounts(t *testing.T) {
	text := strings.Repeat("It was the best of times, it was the worst of times;\nthe age of wisdom... ", 500)
	data := []byte(text)
	want := wc.Count(text)
	for _, n := range []int{1, 2, 3, 7, 50} {
		cuts := split(data, n)
		var got wc.Result
		for i := 0; i < n; i++ {
			wc.Merge(&got, wc.Count(string(data[cuts[i]:cuts[i+1]])))
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("n=%d: merged counts differ from whole-file counts", n)
		}
	}
}

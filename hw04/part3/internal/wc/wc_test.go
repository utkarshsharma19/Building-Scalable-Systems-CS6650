package wc

import (
	"reflect"
	"testing"
)

func TestCount(t *testing.T) {
	got := Count("The cat's hat -- the CAT, 'the' end.\n42 cats")
	want := map[string]int{"the": 3, "cat's": 1, "hat": 1, "cat": 1, "end": 1, "42": 1, "cats": 1}
	if !reflect.DeepEqual(got.Counts, want) || got.TotalWords != 9 || got.UniqueWords != 7 {
		t.Fatalf("got %+v", got)
	}
}

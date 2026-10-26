package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSearchChecksExactly100(t *testing.T) {
	store := newProductStore()
	store.seed(100_000)

	for _, q := range []string{"electro", "zzz-no-match", "Product", "alpha"} {
		for range 50 {
			matches, checked := store.search(q)
			if checked != searchCheckLimit {
				t.Fatalf("q=%q: checked %d products, want %d", q, checked, searchCheckLimit)
			}
			if len(matches) > searchResultLimit {
				t.Fatalf("q=%q: %d matches, want <= %d", q, len(matches), searchResultLimit)
			}
		}
	}
}

func TestSearchMatchesNameAndCategoryCaseInsensitive(t *testing.T) {
	store := newProductStore()
	store.seed(100)

	if m, _ := store.search("ELECTRO"); len(m) != 10 {
		t.Errorf("category match: got %d, want 10", len(m))
	}
	if m, _ := store.search("alpha"); len(m) != 10 {
		t.Errorf("name match: got %d, want 10", len(m))
	}
	if m, _ := store.search("product"); len(m) != searchResultLimit {
		t.Errorf("cap: got %d, want %d", len(m), searchResultLimit)
	}
}

func TestSearchEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newProductStore()
	store.seed(1000)
	r := newRouter(store, slog.New(slog.NewTextHandler(io.Discard, nil)))

	cases := []struct {
		url  string
		code int
	}{
		{"/products/search?q=Electronics", 200},
		{"/products/search", 400},
		{"/products/search?q=", 400},
		{"/products/search?q=" + strings.Repeat("a", 101), 400},
		{"/products/1", 200},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.url, nil))
		if w.Code != tc.code {
			t.Errorf("%s: got %d, want %d", tc.url, w.Code, tc.code)
		}
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/products/search?q=books", nil))
	var body searchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalFound != len(body.Products) || body.TotalFound != 10 || body.SearchTime == "" {
		t.Errorf("unexpected body: total_found=%d products=%d search_time=%q",
			body.TotalFound, len(body.Products), body.SearchTime)
	}
	if got := w.Header().Get("X-Products-Checked"); got != "100" {
		t.Errorf("X-Products-Checked = %q, want 100", got)
	}
}

func BenchmarkSearch(b *testing.B) {
	store := newProductStore()
	store.seed(100_000)
	for b.Loop() {
		store.search("electro")
	}
}

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const body = `{"customer_id":1,"items":[{"product_id":"p1","quantity":1,"price":9.99}]}`

// With capacity 1, N concurrent sync orders must take about N x delay:
// the payment processor really is the bottleneck, even though it only sleeps.
func TestSyncIsSerialized(t *testing.T) {
	const n, delay = 4, 50 * time.Millisecond
	s := &server{payment: make(chan struct{}, 1), paymentDelay: delay}

	start := time.Now()
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			s.syncOrder(rec, httptest.NewRequest(http.MethodPost, "/orders/sync", strings.NewReader(body)))
			if rec.Code != http.StatusOK {
				t.Errorf("status %d", rec.Code)
			}
		}()
	}
	wg.Wait()
	if got := time.Since(start); got < n*delay {
		t.Fatalf("%d orders took %v, want >= %v (not serialized)", n, got, n*delay)
	}
}

func TestValidation(t *testing.T) {
	s := &server{payment: make(chan struct{}, 1)}
	for _, b := range []string{`{}`, `{"customer_id":1,"items":[]}`, `not json`} {
		rec := httptest.NewRecorder()
		s.syncOrder(rec, httptest.NewRequest(http.MethodPost, "/orders/sync", strings.NewReader(b)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%q: status %d, want 400", b, rec.Code)
		}
	}
}

func TestAsyncWithoutSNS(t *testing.T) {
	s := &server{payment: make(chan struct{}, 1)}
	rec := httptest.NewRecorder()
	s.asyncOrder(rec, httptest.NewRequest(http.MethodPost, "/orders/async", strings.NewReader(body)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", rec.Code)
	}
}

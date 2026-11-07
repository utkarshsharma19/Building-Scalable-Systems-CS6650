// Order Receiver: POST /orders/sync (waits for payment) and POST /orders/async
// (publishes to SNS and returns 202 right away).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"hw07/internal/order"
)

type server struct {
	// payment is the simulated payment processor. Its capacity is how many
	// orders it can verify at the same time (1 = one order every 3 s).
	// A sleeping goroutine doesn't block a thread, so without this channel
	// the "bottleneck" would not exist.
	payment      chan struct{}
	paymentDelay time.Duration

	sns      *sns.Client
	topicARN string

	syncWaiting   atomic.Int64 // requests queued for the payment processor
	syncCompleted atomic.Int64
	asyncAccepted atomic.Int64
	asyncFailed   atomic.Int64
}

func main() {
	port := order.EnvString("PORT", "8080")
	order.RunHealthcheckIfAsked(port)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	s := &server{
		payment:      make(chan struct{}, order.EnvInt("PAYMENT_CAPACITY", 1)),
		paymentDelay: order.EnvDuration("PAYMENT_DELAY", 3*time.Second),
		topicARN:     os.Getenv("SNS_TOPIC_ARN"),
	}
	if s.topicARN != "" {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			slog.Error("aws config", "err", err)
			os.Exit(1)
		}
		s.sns = sns.NewFromConfig(cfg)
	} else {
		slog.Warn("SNS_TOPIC_ARN not set: /orders/async will return 503")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /stats", s.stats)
	mux.HandleFunc("POST /orders/sync", s.syncOrder)
	mux.HandleFunc("POST /orders/async", s.asyncOrder)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		slog.Info("receiver listening", "port", port,
			"payment_capacity", cap(s.payment), "payment_delay", s.paymentDelay.String())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen", "err", err)
			os.Exit(1)
		}
	}()
	go s.logStats()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func decode(w http.ResponseWriter, r *http.Request) (order.Request, bool) {
	var req order.Request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "body must be JSON: "+err.Error())
		return req, false
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return req, false
	}
	return req, true
}

// POST /orders/sync: Customer -> API -> Payment (3 s) -> 200.
func (s *server) syncOrder(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	o := order.New(req)

	s.syncWaiting.Add(1)
	select {
	case s.payment <- struct{}{}: // our turn at the payment processor
		s.syncWaiting.Add(-1)
	case <-r.Context().Done(): // the client (or the ALB) gave up while we queued
		s.syncWaiting.Add(-1)
		return
	}
	o.Status = order.StatusProcessing
	order.VerifyPayment(s.paymentDelay)
	<-s.payment

	o.Status = order.StatusCompleted
	s.syncCompleted.Add(1)
	writeJSON(w, http.StatusOK, o)
}

// POST /orders/async: Customer -> API -> SNS -> 202 (< 100 ms).
func (s *server) asyncOrder(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	if s.sns == nil {
		writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "SNS_TOPIC_ARN is not set")
		return
	}
	o := order.New(req)
	body, _ := json.Marshal(o)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if _, err := s.sns.Publish(ctx, &sns.PublishInput{
		TopicArn: aws.String(s.topicARN),
		Message:  aws.String(string(body)),
	}); err != nil {
		s.asyncFailed.Add(1)
		slog.Error("sns publish", "order_id", o.OrderID, "err", err)
		writeError(w, http.StatusServiceUnavailable, "PUBLISH_FAILED", err.Error())
		return
	}
	s.asyncAccepted.Add(1)
	writeJSON(w, http.StatusAccepted, o)
}

func (s *server) snapshot() map[string]int64 {
	return map[string]int64{
		"sync_waiting":   s.syncWaiting.Load(),
		"sync_completed": s.syncCompleted.Load(),
		"async_accepted": s.asyncAccepted.Load(),
		"async_failed":   s.asyncFailed.Load(),
	}
}

func (s *server) stats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.snapshot())
}

func (s *server) logStats() {
	var last map[string]int64
	for range time.Tick(10 * time.Second) {
		cur := s.snapshot()
		if last == nil || cur["sync_completed"] != last["sync_completed"] ||
			cur["async_accepted"] != last["async_accepted"] || cur["sync_waiting"] != 0 {
			slog.Info("stats", "sync_waiting", cur["sync_waiting"], "sync_completed", cur["sync_completed"],
				"async_accepted", cur["async_accepted"], "async_failed", cur["async_failed"])
		}
		last = cur
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, kind, msg string) {
	writeJSON(w, code, map[string]string{"error": kind, "message": msg})
}

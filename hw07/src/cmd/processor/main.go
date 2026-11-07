// Order Processor: long-polls SQS and verifies each order's payment in its own
// goroutine, with at most WORKERS payments running at once.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"hw07/internal/order"
)

type processor struct {
	sqs          *sqs.Client
	queueURL     string
	workers      int
	paymentDelay time.Duration
	// slots is the worker pool: one token per payment in progress.
	slots chan struct{}

	busy      atomic.Int64 // payments running right now
	received  atomic.Int64
	completed atomic.Int64
	failed    atomic.Int64
}

func main() {
	port := order.EnvString("PORT", "8080")
	order.RunHealthcheckIfAsked(port)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	queueURL := os.Getenv("SQS_QUEUE_URL")
	if queueURL == "" {
		slog.Error("SQS_QUEUE_URL is required")
		os.Exit(1)
	}
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		slog.Error("aws config", "err", err)
		os.Exit(1)
	}
	workers := max(1, order.EnvInt("WORKERS", 1))
	p := &processor{
		sqs:          sqs.NewFromConfig(cfg),
		queueURL:     queueURL,
		workers:      workers,
		paymentDelay: order.EnvDuration("PAYMENT_DELAY", 3*time.Second),
		slots:        make(chan struct{}, workers),
	}

	// /health for the ECS health check, /stats for the report.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(p.snapshot())
	})
	go func() { _ = http.ListenAndServe(":"+port, mux) }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go p.logStats(ctx)

	slog.Info("processor started", "workers", workers, "payment_delay", p.paymentDelay.String())
	var wg sync.WaitGroup
	p.pollLoop(ctx, &wg)
	wg.Wait() // let in-flight payments finish so their messages get deleted
}

// pollLoop: ReceiveMessage (long poll, up to 10) -> one goroutine per message -> repeat.
//
// It only asks SQS for as many messages as there are free worker slots. If it
// took 10 messages with no free worker, they would sit here until their 30 s
// visibility timeout ran out and SQS handed them to someone else: duplicate work.
func (p *processor) pollLoop(ctx context.Context, wg *sync.WaitGroup) {
	for ctx.Err() == nil {
		// Block until at least one worker is free, then grab up to 9 more.
		select {
		case p.slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		reserved := 1
	more:
		for reserved < 10 {
			select {
			case p.slots <- struct{}{}:
				reserved++
			default:
				break more
			}
		}

		out, err := p.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(p.queueURL),
			MaxNumberOfMessages: int32(reserved),
			WaitTimeSeconds:     20,
		})
		if err != nil {
			p.release(reserved)
			if errors.Is(err, context.Canceled) {
				return
			}
			slog.Error("receive", "err", err)
			time.Sleep(time.Second)
			continue
		}
		p.release(reserved - len(out.Messages)) // give back unused slots
		p.received.Add(int64(len(out.Messages)))

		for _, m := range out.Messages {
			wg.Add(1)
			go func(m types.Message) {
				defer wg.Done()
				defer p.release(1)
				p.handle(m)
			}(m)
		}
	}
}

func (p *processor) release(n int) {
	for range n {
		<-p.slots
	}
}

func (p *processor) handle(m types.Message) {
	var o order.Order
	if err := json.Unmarshal([]byte(aws.ToString(m.Body)), &o); err != nil {
		// A malformed body will never succeed: delete it instead of retrying forever.
		slog.Error("bad message, deleting", "err", err)
		p.delete(m)
		p.failed.Add(1)
		return
	}
	o.Status = order.StatusProcessing
	p.busy.Add(1)
	order.VerifyPayment(p.paymentDelay)
	p.busy.Add(-1)
	o.Status = order.StatusCompleted

	// Fresh context: finish deleting even while shutting down.
	if p.delete(m) {
		p.completed.Add(1)
	} else {
		p.failed.Add(1) // message comes back after the visibility timeout
	}
}

func (p *processor) delete(m types.Message) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := p.sqs.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(p.queueURL),
		ReceiptHandle: m.ReceiptHandle,
	})
	if err != nil {
		slog.Error("delete", "err", err)
		return false
	}
	return true
}

func (p *processor) snapshot() map[string]int64 {
	return map[string]int64{
		"workers":   int64(p.workers),
		"in_flight": p.busy.Load(),
		"received":  p.received.Load(),
		"completed": p.completed.Load(),
		"failed":    p.failed.Load(),
	}
}

// One log line every 10 s while busy: completed/s is the processing rate.
func (p *processor) logStats(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	var last int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s := p.snapshot()
		if s["completed"] == last && s["in_flight"] == 0 {
			continue
		}
		slog.Info("stats", "workers", s["workers"], "in_flight", s["in_flight"],
			"completed", s["completed"], "rate_per_s", float64(s["completed"]-last)/10, "failed", s["failed"])
		last = s["completed"]
	}
}

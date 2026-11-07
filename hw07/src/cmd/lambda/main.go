// Lambda order processor (Part III): SNS -> Lambda, no SQS. Same 3 s payment.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"hw07/internal/order"
)

var (
	paymentDelay = order.EnvDuration("PAYMENT_DELAY", 3*time.Second)
	coldStart    = true // package-level state survives between warm invocations
)

func handler(ctx context.Context, ev events.SNSEvent) error {
	cold := coldStart
	coldStart = false
	for _, rec := range ev.Records {
		var o order.Order
		if err := json.Unmarshal([]byte(rec.SNS.Message), &o); err != nil {
			// Returning an error makes Lambda retry (twice) and then drop it.
			slog.Error("bad message", "err", err, "message_id", rec.SNS.MessageID)
			return err
		}
		start := time.Now()
		order.VerifyPayment(paymentDelay)
		o.Status = order.StatusCompleted
		slog.Info("order completed", "order_id", o.OrderID, "customer_id", o.CustomerID,
			"cold_start", cold, "payment_ms", time.Since(start).Milliseconds(),
			"queued_ms", time.Since(o.CreatedAt).Milliseconds())
	}
	return nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	lambda.Start(handler)
}

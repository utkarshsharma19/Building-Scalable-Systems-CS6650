// Package order holds the types and the simulated payment step shared by the
// receiver, the ECS processor and the Lambda function.
package order

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

type Item struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Order struct {
	OrderID    string    `json:"order_id"`
	CustomerID int       `json:"customer_id"`
	Status     string    `json:"status"` // pending, processing, completed
	Items      []Item    `json:"items"`
	CreatedAt  time.Time `json:"created_at"`
}

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
)

// Request is what clients POST to /orders/sync and /orders/async.
type Request struct {
	CustomerID int    `json:"customer_id"`
	Items      []Item `json:"items"`
}

func (r Request) Validate() error {
	if r.CustomerID <= 0 {
		return errors.New("customer_id must be a positive integer")
	}
	if len(r.Items) == 0 {
		return errors.New("items must not be empty")
	}
	for _, it := range r.Items {
		if it.Quantity <= 0 {
			return errors.New("every item needs a positive quantity")
		}
	}
	return nil
}

// New builds a pending order with a fresh ID.
func New(r Request) Order {
	return Order{
		OrderID:    NewID(),
		CustomerID: r.CustomerID,
		Status:     StatusPending,
		Items:      r.Items,
		CreatedAt:  time.Now().UTC(),
	}
}

func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// VerifyPayment simulates the external payment processor. It only sleeps;
// callers decide how many may run at once (that is what limits throughput).
func VerifyPayment(d time.Duration) {
	time.Sleep(d)
}

// HW8: shopping-cart API with two interchangeable backends (CART_BACKEND=dynamodb | mysql).
//
// Same API for both (HW5 OpenAPI spec):
//
//	POST /shopping-carts              {"customer_id": 1}               -> 201 {"shopping_cart_id": N}
//	GET  /shopping-carts/{id}                                          -> 200 cart with items
//	POST /shopping-carts/{id}/items   {"product_id": 7, "quantity": 2} -> 204
//	GET  /customers/{id}/shopping-carts                                -> 200 customer history
//
// The handlers only validate input and map errors to status codes; dynamo.go and mysql.go
// implement the cartStore interface. Every request logs one JSON line with the time spent
// inside the database call (db_ms) so in-service latency can be separated from network time.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	maxItemsPerCart = 50
	maxQuantity     = 10000
	opTimeout       = 3 * time.Second
)

var (
	errNotFound = errors.New("not found")
	errCartFull = errors.New("cart full")
	errBusy     = errors.New("database throttled or busy")
)

type cartStore interface {
	Create(ctx context.Context, customerID int64) (int64, error)
	Get(ctx context.Context, id int64, consistent bool) (*cartView, error)
	AddItem(ctx context.Context, cartID, productID, qty int64) error
	History(ctx context.Context, customerID int64) ([]cartView, error)
}

type cartItem struct {
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

type cartView struct {
	ShoppingCartID int        `json:"shopping_cart_id"`
	CustomerID     int        `json:"customer_id"`
	Items          []cartItem `json:"items"`
	ItemCount      int        `json:"item_count"`
	TotalQuantity  int        `json:"total_quantity"`
	Version        int        `json:"version"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
}

type apiError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

type server struct {
	store   cartStore
	backend string
	log     *slog.Logger
}

func main() {
	health := flag.Bool("healthcheck", false, "probe /health and exit (for container health checks)")
	flag.Parse()
	port := envOr("PORT", "8080")
	if *health {
		resp, err := http.Get("http://127.0.0.1:" + port + "/health")
		if err != nil || resp.StatusCode != 200 {
			os.Exit(1)
		}
		os.Exit(0)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()
	s := &server{backend: envOr("CART_BACKEND", "dynamodb"), log: log}
	mux := http.NewServeMux()

	switch s.backend {
	case "dynamodb":
		ds, err := newDynamoStore(ctx, log)
		if err != nil {
			log.Error("dynamodb init", "err", err)
			os.Exit(1)
		}
		s.store = ds
		if os.Getenv("ENABLE_DEBUG") == "true" {
			mux.HandleFunc("POST /debug/consistency-probe", ds.consistencyProbe)
		}
	case "mysql":
		ms, err := newMySQLStore(ctx, log)
		if err != nil {
			log.Error("mysql init", "err", err)
			os.Exit(1)
		}
		s.store = ms
	default:
		log.Error("unknown CART_BACKEND", "backend", s.backend)
		os.Exit(1)
	}

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "backend": s.backend})
	})
	mux.HandleFunc("POST /shopping-carts", s.timed("create_cart", s.createCart))
	mux.HandleFunc("GET /shopping-carts/{id}", s.timed("get_cart", s.getCart))
	mux.HandleFunc("POST /shopping-carts/{id}/items", s.timed("add_items", s.addItem))
	mux.HandleFunc("GET /customers/{id}/shopping-carts", s.timed("customer_history", s.customerHistory))

	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Info("listening", "port", port, "backend", s.backend)
	if err := srv.ListenAndServe(); err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}

// ---------- handlers ----------

func (s *server) createCart(w http.ResponseWriter, r *http.Request) (time.Duration, int) {
	var body struct {
		CustomerID *int64 `json:"customer_id"`
	}
	if err := decode(r, &body); err != nil {
		return 0, writeErr(w, 400, "INVALID_INPUT", "request body must be JSON with customer_id", err.Error())
	}
	if body.CustomerID == nil || *body.CustomerID < 1 || *body.CustomerID > 2147483647 {
		return 0, writeErr(w, 400, "INVALID_INPUT", "customer_id must be an integer between 1 and 2147483647", "")
	}
	ctx, cancel := context.WithTimeout(r.Context(), opTimeout)
	defer cancel()
	t0 := time.Now()
	id, err := s.store.Create(ctx, *body.CustomerID)
	dbTime := time.Since(t0)
	if err != nil {
		return dbTime, s.storeErr(w, err)
	}
	w.Header().Set("Location", fmt.Sprintf("/shopping-carts/%d", id))
	return dbTime, writeJSON(w, 201, map[string]any{"shopping_cart_id": id, "customer_id": *body.CustomerID, "items": []cartItem{}})
}

func (s *server) getCart(w http.ResponseWriter, r *http.Request) (time.Duration, int) {
	id, ok := pathID(r)
	if !ok {
		return 0, writeErr(w, 400, "INVALID_INPUT", "shopping cart id must be a positive integer", "")
	}
	ctx, cancel := context.WithTimeout(r.Context(), opTimeout)
	defer cancel()
	t0 := time.Now()
	cart, err := s.store.Get(ctx, id, r.URL.Query().Get("consistent") == "true")
	dbTime := time.Since(t0)
	if err != nil {
		return dbTime, s.storeErr(w, err)
	}
	return dbTime, writeJSON(w, 200, cart)
}

func (s *server) addItem(w http.ResponseWriter, r *http.Request) (time.Duration, int) {
	id, ok := pathID(r)
	if !ok {
		return 0, writeErr(w, 400, "INVALID_INPUT", "shopping cart id must be a positive integer", "")
	}
	var body struct {
		ProductID *int64 `json:"product_id"`
		Quantity  *int64 `json:"quantity"`
	}
	if err := decode(r, &body); err != nil {
		return 0, writeErr(w, 400, "INVALID_INPUT", "request body must be JSON with product_id and quantity", err.Error())
	}
	if body.ProductID == nil || *body.ProductID < 1 || *body.ProductID > 2147483647 {
		return 0, writeErr(w, 400, "INVALID_INPUT", "product_id must be a positive int32", "")
	}
	if body.Quantity == nil || *body.Quantity < 1 || *body.Quantity > maxQuantity {
		return 0, writeErr(w, 400, "INVALID_INPUT", fmt.Sprintf("quantity must be between 1 and %d", maxQuantity), "")
	}
	ctx, cancel := context.WithTimeout(r.Context(), opTimeout)
	defer cancel()
	t0 := time.Now()
	err := s.store.AddItem(ctx, id, *body.ProductID, *body.Quantity)
	dbTime := time.Since(t0)
	if err != nil {
		return dbTime, s.storeErr(w, err)
	}
	w.WriteHeader(204)
	return dbTime, 204
}

func (s *server) customerHistory(w http.ResponseWriter, r *http.Request) (time.Duration, int) {
	id, ok := pathID(r)
	if !ok {
		return 0, writeErr(w, 400, "INVALID_INPUT", "customer id must be a positive integer", "")
	}
	ctx, cancel := context.WithTimeout(r.Context(), opTimeout)
	defer cancel()
	t0 := time.Now()
	carts, err := s.store.History(ctx, id)
	dbTime := time.Since(t0)
	if err != nil {
		return dbTime, s.storeErr(w, err)
	}
	return dbTime, writeJSON(w, 200, map[string]any{"customer_id": id, "shopping_carts": carts})
}

// storeErr maps store errors to HTTP. The underlying database error is logged, never sent to the client.
func (s *server) storeErr(w http.ResponseWriter, err error) int {
	switch {
	case errors.Is(err, errNotFound):
		return writeErr(w, 404, "NOT_FOUND", "shopping cart not found", "")
	case errors.Is(err, errCartFull):
		return writeErr(w, 409, "CART_FULL", fmt.Sprintf("cart already has %d distinct products", maxItemsPerCart), "")
	case errors.Is(err, errBusy):
		s.log.Warn("database busy", "err", err)
		return writeErr(w, 503, "THROTTLED", "database is busy, retry later", "")
	case errors.Is(err, context.DeadlineExceeded):
		s.log.Error("timeout", "err", err)
		return writeErr(w, 504, "DB_TIMEOUT", "database call timed out", "")
	default:
		s.log.Error("database error", "err", err)
		return writeErr(w, 500, "INTERNAL_ERROR", "internal server error", "")
	}
}

// timed logs one JSON line per request with total and in-database time.
func (s *server) timed(op string, h func(http.ResponseWriter, *http.Request) (time.Duration, int)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		db, status := h(w, r)
		s.log.Info("request", "op", op, "status", status, "total_ms", ms(time.Since(start)), "db_ms", ms(db), "path", r.URL.Path)
	}
}

// ---------- helpers ----------

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<16)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 32)
	return id, err == nil && id >= 1
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func writeJSON(w http.ResponseWriter, status int, v any) int {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
	return status
}

func writeErr(w http.ResponseWriter, status int, code, msg, details string) int {
	return writeJSON(w, status, apiError{Error: code, Message: msg, Details: details})
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

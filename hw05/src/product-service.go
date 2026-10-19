package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Product struct {
	ProductID    int32  `json:"product_id"`
	SKU          string `json:"sku"`
	Manufacturer string `json:"manufacturer"`
	CategoryID   int32  `json:"category_id"`
	Weight       int32  `json:"weight"`
	SomeOtherID  int32  `json:"some_other_id"`
}

// productInput mirrors Product with pointer fields so a missing required
// field (nil) can be told apart from a zero value (weight: 0 is valid).
// int32 targets make encoding/json reject floats, strings and overflow.
type productInput struct {
	ProductID    *int32  `json:"product_id"`
	SKU          *string `json:"sku"`
	Manufacturer *string `json:"manufacturer"`
	CategoryID   *int32  `json:"category_id"`
	Weight       *int32  `json:"weight"`
	SomeOtherID  *int32  `json:"some_other_id"`
}

type APIError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

type productStore struct {
	mu       sync.RWMutex
	products map[int32]Product
}

func newProductStore() *productStore {
	return &productStore{products: make(map[int32]Product)}
}

func (s *productStore) get(id int32) (Product, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.products[id]
	return p, ok
}

func (s *productStore) update(p Product) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.products[p.ProductID]; !ok {
		return false
	}
	s.products[p.ProductID] = p
	return true
}

func (s *productStore) put(p Product) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.products[p.ProductID] = p
}

func (s *productStore) seed(n int) {
	for i := 1; i <= n; i++ {
		s.put(Product{
			ProductID:    int32(i),
			SKU:          fmt.Sprintf("SKU-%05d", i),
			Manufacturer: "Acme Corporation",
			CategoryID:   int32(i%50 + 1),
			Weight:       int32(100 + i%5000),
			SomeOtherID:  int32(i%1000 + 1),
		})
	}
}

var (
	requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests (Rate).",
	}, []string{"method", "endpoint", "status"})

	errorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_errors_total",
		Help: "Total HTTP responses with a non-2xx status (Errors).",
	}, []string{"method", "endpoint", "status"})

	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Server-side request latency (Duration).",
		Buckets: []float64{.0001, .00025, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
	}, []string{"method", "endpoint"})
)

func observe(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if p := c.Request.URL.Path; p == "/metrics" || p == "/health" {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		elapsed := time.Since(start)

		// Use the route template (/products/:productId), not the raw path,
		// so metric label cardinality stays bounded.
		endpoint := c.FullPath()
		if endpoint == "" {
			endpoint = "unmatched"
		}
		method := c.Request.Method
		status := c.Writer.Status()
		code := strconv.Itoa(status)

		requestsTotal.WithLabelValues(method, endpoint, code).Inc()
		if status < 200 || status > 299 {
			errorsTotal.WithLabelValues(method, endpoint, code).Inc()
		}
		requestDuration.WithLabelValues(method, endpoint).Observe(elapsed.Seconds())

		logger.Info("request",
			"method", method,
			"endpoint", endpoint,
			"path", c.Request.URL.Path,
			"status", status,
			"duration_ms", float64(elapsed.Microseconds())/1000.0,
			"client_ip", c.ClientIP(),
		)
	}
}

func writeError(c *gin.Context, status int, code, message, details string) {
	c.AbortWithStatusJSON(status, APIError{Error: code, Message: message, Details: details})
}

func parseProductID(raw string) (int32, error) {
	id, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, errors.New("product ID must be an integer in the int32 range")
	}
	if id < 1 {
		return 0, errors.New("product ID must be a positive integer")
	}
	return int32(id), nil
}

func simulateFailure(c *gin.Context) {
	if c.GetHeader("X-Simulate-Error") == "true" {
		panic("simulated internal failure")
	}
}

func getProduct(store *productStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		simulateFailure(c)

		id, err := parseProductID(c.Param("productId"))
		if err != nil {
			writeError(c, http.StatusNotFound, "NOT_FOUND", "Product not found", err.Error())
			return
		}
		p, ok := store.get(id)
		if !ok {
			writeError(c, http.StatusNotFound, "NOT_FOUND", "Product not found",
				fmt.Sprintf("no product with ID %d", id))
			return
		}
		c.JSON(http.StatusOK, p)
	}
}

func addProductDetails(store *productStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		simulateFailure(c)

		id, err := parseProductID(c.Param("productId"))
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_INPUT", "Invalid product ID", err.Error())
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		var in productInput
		if err := json.NewDecoder(c.Request.Body).Decode(&in); err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_INPUT", "Request body is not a valid Product", err.Error())
			return
		}
		p, err := validateProduct(in)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_INPUT", "The provided input data is invalid", err.Error())
			return
		}
		if p.ProductID != id {
			writeError(c, http.StatusBadRequest, "INVALID_INPUT", "The provided input data is invalid",
				fmt.Sprintf("body product_id %d does not match path productId %d", p.ProductID, id))
			return
		}

		if !store.update(p) {
			writeError(c, http.StatusNotFound, "NOT_FOUND", "Product not found",
				fmt.Sprintf("no product with ID %d", id))
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func validateProduct(in productInput) (Product, error) {
	switch {
	case in.ProductID == nil:
		return Product{}, errors.New("product_id is required")
	case in.SKU == nil:
		return Product{}, errors.New("sku is required")
	case in.Manufacturer == nil:
		return Product{}, errors.New("manufacturer is required")
	case in.CategoryID == nil:
		return Product{}, errors.New("category_id is required")
	case in.Weight == nil:
		return Product{}, errors.New("weight is required")
	case in.SomeOtherID == nil:
		return Product{}, errors.New("some_other_id is required")
	}

	if *in.ProductID < 1 {
		return Product{}, errors.New("product_id must be >= 1")
	}
	if n := utf8.RuneCountInString(*in.SKU); n < 1 || n > 100 {
		return Product{}, errors.New("sku must be 1-100 characters")
	}
	if n := utf8.RuneCountInString(*in.Manufacturer); n < 1 || n > 200 {
		return Product{}, errors.New("manufacturer must be 1-200 characters")
	}
	if *in.CategoryID < 1 {
		return Product{}, errors.New("category_id must be >= 1")
	}
	if *in.Weight < 0 {
		return Product{}, errors.New("weight must be >= 0")
	}
	if *in.SomeOtherID < 1 {
		return Product{}, errors.New("some_other_id must be >= 1")
	}

	return Product{
		ProductID:    *in.ProductID,
		SKU:          *in.SKU,
		Manufacturer: *in.Manufacturer,
		CategoryID:   *in.CategoryID,
		Weight:       *in.Weight,
		SomeOtherID:  *in.SomeOtherID,
	}, nil
}

func newRouter(store *productStore, logger *slog.Logger) *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	// observe runs first so it also records requests that end in a panic.
	r.Use(observe(logger))
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, err any) {
		logger.Error("panic recovered", "error", fmt.Sprint(err), "path", c.Request.URL.Path)
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", "")
	}))

	r.GET("/products/:productId", getProduct(store))
	r.POST("/products/:productId/details", addProductDetails(store))

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	return r
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	store := newProductStore()
	seedCount := envInt("SEED_PRODUCTS", 1000)
	store.seed(seedCount)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info("starting product service", "port", port, "seeded_products", seedCount)
	if err := newRouter(store, logger).Run(":" + port); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

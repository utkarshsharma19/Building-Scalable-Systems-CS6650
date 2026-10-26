package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Product struct {
	ProductID    int32  `json:"product_id"`
	Name         string `json:"name"`
	SKU          string `json:"sku"`
	Manufacturer string `json:"manufacturer"`
	CategoryID   int32  `json:"category_id"`
	Weight       int32  `json:"weight"`
	SomeOtherID  int32  `json:"some_other_id"`
}

// Pointer fields tell a missing field apart from a zero value. name is optional.
type productInput struct {
	ProductID    *int32  `json:"product_id"`
	Name         *string `json:"name"`
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

// Category ID i maps to categories[i-1].
var categories = []string{
	"Electronics", "Books", "Home", "Garden", "Toys",
	"Sports", "Clothing", "Beauty", "Automotive", "Grocery",
}

var brands = []string{
	"Alpha", "Beta", "Gamma", "Delta", "Epsilon",
	"Zeta", "Eta", "Theta", "Iota", "Kappa",
}

func categoryName(id int32) string {
	if id < 1 || int(id) > len(categories) {
		return ""
	}
	return categories[id-1]
}

type productStore struct {
	products sync.Map
	size     atomic.Int32
}

func newProductStore() *productStore {
	return &productStore{}
}

func (s *productStore) get(id int32) (Product, bool) {
	v, ok := s.products.Load(id)
	if !ok {
		return Product{}, false
	}
	return v.(Product), true
}

func (s *productStore) update(p Product) bool {
	old, ok := s.get(p.ProductID)
	if !ok {
		return false
	}
	if p.Name == "" {
		p.Name = old.Name
	}
	s.products.Store(p.ProductID, p)
	return true
}

func (s *productStore) seed(n int) {
	for i := 1; i <= n; i++ {
		brand := brands[(i/len(categories))%len(brands)]
		s.products.Store(int32(i), Product{
			ProductID:    int32(i),
			Name:         fmt.Sprintf("Product %s %d", brand, i),
			SKU:          fmt.Sprintf("SKU-%06d", i),
			Manufacturer: brand,
			CategoryID:   int32(i%len(categories) + 1),
			Weight:       int32(100 + i%5000),
			SomeOtherID:  int32(i%1000 + 1),
		})
	}
	s.size.Store(int32(n))
}

const (
	searchCheckLimit  = 100
	searchResultLimit = 20
)

// search checks exactly 100 products from a random start and returns up to 20 matches.
func (s *productStore) search(query string) (matches []Product, checked int) {
	size := int(s.size.Load())
	if size == 0 {
		return []Product{}, 0
	}
	q := strings.ToLower(query)
	limit := min(searchCheckLimit, size)
	start := rand.IntN(size)
	matches = make([]Product, 0, searchResultLimit)

	for i := 0; i < limit; i++ {
		id := int32((start+i)%size + 1)
		p, ok := s.get(id)
		if !ok {
			continue
		}
		checked++
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(categoryName(p.CategoryID)), q) {
			if len(matches) < searchResultLimit {
				matches = append(matches, p)
			}
		}
	}
	return matches, checked
}

type searchResponse struct {
	Products   []Product `json:"products"`
	TotalFound int       `json:"total_found"`
	SearchTime string    `json:"search_time"`
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
		Buckets: []float64{.0001, .00025, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
	}, []string{"method", "endpoint"})

	requestsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Requests currently being handled (Saturation).",
	})

	searchChecked = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "search_products_checked",
		Help:    "Products checked per search (should always be 100).",
		Buckets: []float64{50, 99, 100, 101, 200},
	})
)

func observe(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if p := c.Request.URL.Path; p == "/metrics" || p == "/health" {
			c.Next()
			return
		}
		requestsInFlight.Inc()
		start := time.Now()
		c.Next()
		elapsed := time.Since(start)
		requestsInFlight.Dec()

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

	var name string
	if in.Name != nil {
		if n := utf8.RuneCountInString(*in.Name); n < 1 || n > 200 {
			return Product{}, errors.New("name must be 1-200 characters")
		}
		name = *in.Name
	}

	return Product{
		ProductID:    *in.ProductID,
		Name:         name,
		SKU:          *in.SKU,
		Manufacturer: *in.Manufacturer,
		CategoryID:   *in.CategoryID,
		Weight:       *in.Weight,
		SomeOtherID:  *in.SomeOtherID,
	}, nil
}

func searchProducts(store *productStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		simulateFailure(c)

		q, ok := c.GetQuery("q")
		if !ok {
			writeError(c, http.StatusBadRequest, "INVALID_INPUT", "Missing search query", "query parameter q is required")
			return
		}
		if n := utf8.RuneCountInString(q); n < 1 || n > 100 {
			writeError(c, http.StatusBadRequest, "INVALID_INPUT", "Invalid search query", "q must be 1-100 characters")
			return
		}

		start := time.Now()
		matches, checked := store.search(q)
		elapsed := time.Since(start)
		searchChecked.Observe(float64(checked))

		c.Header("X-Products-Checked", strconv.Itoa(checked))
		c.JSON(http.StatusOK, searchResponse{
			Products:   matches,
			TotalFound: len(matches),
			SearchTime: elapsed.String(),
		})
	}
}

func newRouter(store *productStore, logger *slog.Logger) *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(observe(logger))
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, err any) {
		logger.Error("panic recovered", "error", fmt.Sprint(err), "path", c.Request.URL.Path)
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", "")
	}))

	r.GET("/products/search", searchProducts(store))
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
	seedCount := envInt("SEED_PRODUCTS", 100_000)
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

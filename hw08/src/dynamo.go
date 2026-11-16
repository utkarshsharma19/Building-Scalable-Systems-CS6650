// DynamoDB backend (STEP II).
//
// Table design: one table, partition key cart_id (Number), no sort key. Items are embedded in
// the cart as a map product_id -> quantity, so every endpoint is a single-item operation
// (PutItem / GetItem / UpdateItem) on one key. A GSI on (customer_id, created_at) serves
// customer history.
package main

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
)

const cartTTL = 30 * 24 * time.Hour // carts are session data: DynamoDB TTL deletes stale ones for free

type dynamoStore struct {
	db          *dynamodb.Client
	table       string
	customerIdx string
	log         *slog.Logger
}

func newDynamoStore(ctx context.Context, log *slog.Logger) (*dynamoStore, error) {
	// "Connection pooling" for DynamoDB = the SDK's HTTP keep-alive pool. DynamoDB is an HTTPS
	// API, so reusing warm TLS connections keeps per-request latency low. The SDK default keeps
	// only 10 idle connections per host; under bursts every request beyond that opens (and later
	// throws away) a new TLS connection. HTTP_MAX_IDLE_PER_HOST lets the load test compare.
	idle, _ := strconv.Atoi(envOr("HTTP_MAX_IDLE_PER_HOST", "200"))
	httpClient := awshttp.NewBuildableClient().WithTransportOptions(func(t *http.Transport) {
		t.MaxIdleConns = max(idle, 100)
		t.MaxIdleConnsPerHost = idle
		t.IdleConnTimeout = 90 * time.Second
		t.DialContext = (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	})
	cfg, err := config.LoadDefaultConfig(ctx, config.WithHTTPClient(httpClient), config.WithRetryMaxAttempts(3))
	if err != nil {
		return nil, err
	}
	db := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		if ep := os.Getenv("DYNAMODB_ENDPOINT"); ep != "" { // DynamoDB Local for development
			o.BaseEndpoint = aws.String(ep)
		}
	})
	s := &dynamoStore{db: db, table: envOr("TABLE_NAME", "hw8-shopping-carts"), customerIdx: envOr("CUSTOMER_INDEX", "customer_id-index"), log: log}
	// Warm the connection pool so the first real request doesn't pay the TLS handshake.
	if _, err := db.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: &s.table}); err != nil {
		log.Warn("describe table at startup failed", "err", err)
	}
	log.Info("dynamodb store ready", "table", s.table, "max_idle_conns_per_host", idle)
	return s, nil
}

// Create: random 31-bit IDs keep the int32 API contract and spread writes evenly across
// partitions (a sequential counter would be one hot item). attribute_not_exists makes a
// collision fail instead of overwriting; we retry with a new ID.
func (s *dynamoStore) Create(ctx context.Context, customerID int64) (int64, error) {
	now := time.Now().UTC()
	for attempt := 0; attempt < 5; attempt++ {
		id := int64(rand.Int32N(2147483646) + 1)
		_, err := s.db.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: &s.table,
			Item: map[string]types.AttributeValue{
				"cart_id":     num(id),
				"customer_id": num(customerID),
				"items":       &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}},
				"version":     num(1),
				"created_at":  str(now.Format(time.RFC3339Nano)),
				"updated_at":  str(now.Format(time.RFC3339Nano)),
				"expires_at":  num(now.Add(cartTTL).Unix()),
			},
			ConditionExpression: aws.String("attribute_not_exists(cart_id)"),
		})
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			s.log.Warn("cart id collision, retrying", "id", id)
			continue
		}
		if err != nil {
			return 0, s.wrap(err)
		}
		return id, nil
	}
	return 0, errors.New("could not allocate a cart id")
}

// Get: eventually consistent by default (half the read cost); consistent=true for strong reads.
func (s *dynamoStore) Get(ctx context.Context, id int64, consistent bool) (*cartView, error) {
	out, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      &s.table,
		Key:            map[string]types.AttributeValue{"cart_id": num(id)},
		ConsistentRead: aws.Bool(consistent),
	})
	if err != nil {
		return nil, s.wrap(err)
	}
	if out.Item == nil {
		return nil, errNotFound
	}
	v := toView(out.Item)
	return &v, nil
}

// AddItem: one atomic UpdateItem adds to the product's quantity (creating the entry if new),
// bumps the version, and refuses if the cart doesn't exist or already has 50 distinct products.
// No read-modify-write, so concurrent adds from many clients never lose updates.
func (s *dynamoStore) AddItem(ctx context.Context, id, productID, qty int64) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                &s.table,
		Key:                      map[string]types.AttributeValue{"cart_id": num(id)},
		UpdateExpression:         aws.String("SET #items.#pid = if_not_exists(#items.#pid, :zero) + :q, updated_at = :now, version = version + :one"),
		ConditionExpression:      aws.String("attribute_exists(cart_id) AND (attribute_exists(#items.#pid) OR size(#items) < :max)"),
		ExpressionAttributeNames: map[string]string{"#items": "items", "#pid": strconv.FormatInt(productID, 10)},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":zero": num(0), ":q": num(qty), ":one": num(1), ":max": num(maxItemsPerCart), ":now": str(now),
		},
		ReturnValuesOnConditionCheckFailure: types.ReturnValuesOnConditionCheckFailureAllOld,
	})
	var ccf *types.ConditionalCheckFailedException
	if errors.As(err, &ccf) {
		if len(ccf.Item) == 0 {
			return errNotFound
		}
		return errCartFull
	}
	return s.wrap(err)
}

func (s *dynamoStore) History(ctx context.Context, customerID int64) ([]cartView, error) {
	out, err := s.db.Query(ctx, &dynamodb.QueryInput{
		TableName:                 &s.table,
		IndexName:                 &s.customerIdx,
		KeyConditionExpression:    aws.String("customer_id = :c"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":c": num(customerID)},
		ScanIndexForward:          aws.Bool(false), // newest first (sort key created_at)
		Limit:                     aws.Int32(100),
	})
	if err != nil {
		return nil, s.wrap(err)
	}
	carts := make([]cartView, 0, len(out.Items))
	for _, it := range out.Items {
		carts = append(carts, toView(it))
	}
	return carts, nil
}

// wrap turns throttling into errBusy; everything else stays a 500 (logged by the handler).
func (s *dynamoStore) wrap(err error) error {
	if err == nil {
		return nil
	}
	var thr *types.ProvisionedThroughputExceededException
	var apiErr smithy.APIError
	if errors.As(err, &thr) || (errors.As(err, &apiErr) && apiErr.ErrorCode() == "ThrottlingException") {
		return errors.Join(errBusy, err)
	}
	return err
}

// consistencyProbe runs write-then-immediately-read trials *inside the region*, so the read
// lands microseconds after the write is acknowledged (a client over the internet can't get that
// close). It counts how often an eventually consistent read is stale and how long until it's fresh.
func (s *dynamoStore) consistencyProbe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Trials int    `json:"trials"`
		Mode   string `json:"mode"` // "create" or "update"
	}
	if err := decode(r, &req); err != nil || req.Trials < 1 || req.Trials > 2000 || (req.Mode != "create" && req.Mode != "update") {
		writeErr(w, 400, "INVALID_INPUT", `body: {"trials": 1..2000, "mode": "create"|"update"}`, "")
		return
	}
	ctx := r.Context()
	type trial struct {
		Stale        bool    `json:"stale"`
		StaleReads   int     `json:"stale_reads"`
		ConvergeMs   float64 `json:"converge_ms"`
		WriteMs      float64 `json:"write_ms"`
		FirstReadMs  float64 `json:"first_read_ms"`
		CartID       int32   `json:"cart_id"`
		WrittenVer   int     `json:"written_version,omitempty"`
		FirstReadVer int     `json:"first_read_version,omitempty"`
	}
	fail := func(err error) {
		s.log.Error("probe", "err", err)
		writeErr(w, 500, "INTERNAL_ERROR", "probe failed", "")
	}
	newCart := func(id int32) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := s.db.PutItem(ctx, &dynamodb.PutItemInput{TableName: &s.table, Item: map[string]types.AttributeValue{
			"cart_id": num(int64(id)), "customer_id": num(999999), "items": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}},
			"version": num(1), "created_at": str(now), "updated_at": str(now), "expires_at": num(time.Now().Add(24 * time.Hour).Unix()),
		}})
		return err
	}
	results := make([]trial, 0, req.Trials)
	var cartID int32
	ver := 1
	if req.Mode == "update" { // one cart, updated repeatedly
		cartID = rand.Int32N(2147483646) + 1
		if err := newCart(cartID); err != nil {
			fail(err)
			return
		}
	}
	for i := 0; i < req.Trials; i++ {
		var t trial
		tw := time.Now()
		if req.Mode == "create" {
			cartID = rand.Int32N(2147483646) + 1
			if err := newCart(cartID); err != nil {
				fail(err)
				return
			}
		} else {
			out, err := s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
				TableName: &s.table, Key: map[string]types.AttributeValue{"cart_id": num(int64(cartID))},
				UpdateExpression:          aws.String("SET version = version + :one, #items.#p = if_not_exists(#items.#p, :zero) + :one"),
				ExpressionAttributeNames:  map[string]string{"#items": "items", "#p": "1"},
				ExpressionAttributeValues: map[string]types.AttributeValue{":one": num(1), ":zero": num(0)},
				ReturnValues:              types.ReturnValueUpdatedNew,
			})
			if err != nil {
				fail(err)
				return
			}
			ver = intAttr(out.Attributes["version"])
			t.WrittenVer = ver
		}
		t.WriteMs = ms(time.Since(tw))
		t.CartID = cartID
		ackAt := time.Now()
		for attempt := 0; ; attempt++ {
			tr := time.Now()
			out, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{TableName: &s.table, Key: map[string]types.AttributeValue{"cart_id": num(int64(cartID))}})
			if err != nil {
				fail(err)
				return
			}
			if attempt == 0 {
				t.FirstReadMs = ms(time.Since(tr))
				if req.Mode == "update" && out.Item != nil {
					t.FirstReadVer = intAttr(out.Item["version"])
				}
			}
			if out.Item != nil && (req.Mode == "create" || intAttr(out.Item["version"]) >= ver) {
				t.ConvergeMs = ms(time.Since(ackAt))
				break
			}
			t.Stale = true
			t.StaleReads++
			if time.Since(ackAt) > 5*time.Second {
				t.ConvergeMs = -1
				break
			}
		}
		results = append(results, t)
	}
	stale := 0
	for _, t := range results {
		if t.Stale {
			stale++
		}
	}
	writeJSON(w, 200, map[string]any{"mode": req.Mode, "trials": req.Trials, "stale_first_reads": stale, "results": results})
}

func toView(it map[string]types.AttributeValue) cartView {
	v := cartView{
		ShoppingCartID: intAttr(it["cart_id"]),
		CustomerID:     intAttr(it["customer_id"]),
		Version:        intAttr(it["version"]),
		CreatedAt:      strAttr(it["created_at"]),
		UpdatedAt:      strAttr(it["updated_at"]),
		Items:          []cartItem{},
	}
	if m, ok := it["items"].(*types.AttributeValueMemberM); ok {
		for pid, q := range m.Value {
			p, _ := strconv.Atoi(pid)
			qty := intAttr(q)
			v.Items = append(v.Items, cartItem{ProductID: p, Quantity: qty})
			v.TotalQuantity += qty
		}
	}
	sort.Slice(v.Items, func(i, j int) bool { return v.Items[i].ProductID < v.Items[j].ProductID })
	v.ItemCount = len(v.Items)
	return v
}

func num(n int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(n, 10)}
}
func str(s string) types.AttributeValue { return &types.AttributeValueMemberS{Value: s} }
func intAttr(a types.AttributeValue) int {
	if n, ok := a.(*types.AttributeValueMemberN); ok {
		v, _ := strconv.Atoi(n.Value)
		return v
	}
	return 0
}
func strAttr(a types.AttributeValue) string {
	if s, ok := a.(*types.AttributeValueMemberS); ok {
		return s.Value
	}
	return ""
}

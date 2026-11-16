// MySQL backend (STEP I), Amazon RDS MySQL 8.0.
//
// Schema (created at startup, RDS is private so there is no other way in):
//
//	shopping_carts (cart_id INT AUTO_INCREMENT PK, customer_id, version, created_at, updated_at,
//	                INDEX idx_customer_created (customer_id, created_at))   -- customer history
//	cart_items     (cart_id, product_id, quantity > 0, added_at, PRIMARY KEY (cart_id, product_id),
//	                FK cart_id -> shopping_carts ON DELETE CASCADE)
//
// A cart's items are one primary-key range read (InnoDB clusters rows by PK), so GET is a PK
// lookup on shopping_carts LEFT JOIN a PK range on cart_items. No ORDER BY in SQL (it forces a
// filesort over the join); items are sorted in Go.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
)

var schema = []string{
	`CREATE TABLE IF NOT EXISTS shopping_carts (
		cart_id     INT UNSIGNED NOT NULL AUTO_INCREMENT,
		customer_id INT UNSIGNED NOT NULL,
		version     INT UNSIGNED NOT NULL DEFAULT 1,
		created_at  DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		updated_at  DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		PRIMARY KEY (cart_id),
		INDEX idx_customer_created (customer_id, created_at)
	) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS cart_items (
		cart_id    INT UNSIGNED NOT NULL,
		product_id INT UNSIGNED NOT NULL,
		quantity   INT UNSIGNED NOT NULL,
		added_at   DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		PRIMARY KEY (cart_id, product_id),
		CONSTRAINT fk_items_cart FOREIGN KEY (cart_id) REFERENCES shopping_carts (cart_id) ON DELETE CASCADE,
		CONSTRAINT chk_quantity CHECK (quantity > 0)
	) ENGINE=InnoDB`,
}

type mysqlStore struct {
	db  *sql.DB
	log *slog.Logger
}

func newMySQLStore(ctx context.Context, log *slog.Logger) (*mysqlStore, error) {
	cfg := mysql.NewConfig()
	cfg.User = envOr("DB_USER", "cartadmin")
	cfg.Passwd = envOr("DB_PASSWORD", "")
	cfg.Net = "tcp"
	cfg.Addr = envOr("DB_ADDR", "127.0.0.1:3306") // host:port (RDS's "endpoint" output already has the port)
	cfg.DBName = envOr("DB_NAME", "carts")
	cfg.ParseTime = true
	cfg.Timeout = 3 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	// Connection pool: POOL_SIZE per task, all kept idle so bursts reuse warm connections.
	// tasks x POOL_SIZE must stay under RDS max_connections (db.t3.micro: about 60-80).
	pool, _ := strconv.Atoi(envOr("POOL_SIZE", "10"))
	db.SetMaxOpenConns(pool)
	db.SetMaxIdleConns(pool)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	// RDS can still be starting when the first task boots: retry for up to ~2 minutes.
	for i := 0; ; i++ {
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = db.PingContext(pctx)
		cancel()
		if err == nil {
			break
		}
		if i == 40 {
			return nil, fmt.Errorf("mysql not reachable: %w", err)
		}
		log.Warn("waiting for mysql", "err", err)
		time.Sleep(3 * time.Second)
	}
	for _, stmt := range schema { // the driver runs one statement per Exec
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return nil, fmt.Errorf("schema: %w", err)
		}
	}
	var maxConn int
	_ = db.QueryRowContext(ctx, "SELECT @@max_connections").Scan(&maxConn)
	log.Info("mysql store ready", "addr", cfg.Addr, "pool_size", pool, "max_connections", maxConn)

	s := &mysqlStore{db: db, log: log}
	go s.logPoolStats()
	return s, nil
}

// logPoolStats prints database/sql pool stats every 10 s (open, in use, waits) so pool sizing
// can be judged from CloudWatch Logs instead of guessed.
func (s *mysqlStore) logPoolStats() {
	for range time.Tick(10 * time.Second) {
		st := s.db.Stats()
		s.log.Info("pool", "open_conns", st.OpenConnections, "in_use", st.InUse, "idle", st.Idle,
			"wait_count", st.WaitCount, "wait_ms", st.WaitDuration.Milliseconds(), "max_open", st.MaxOpenConnections)
	}
}

func (s *mysqlStore) Create(ctx context.Context, customerID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, "INSERT INTO shopping_carts (customer_id) VALUES (?)", customerID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *mysqlStore) Get(ctx context.Context, id int64, _ bool) (*cartView, error) {
	// MySQL has one primary: every read sees the last commit, so "consistent" doesn't apply.
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.cart_id, c.customer_id, c.version, c.created_at, c.updated_at, i.product_id, i.quantity
		FROM shopping_carts c LEFT JOIN cart_items i ON i.cart_id = c.cart_id
		WHERE c.cart_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var v *cartView
	for rows.Next() {
		var cid, cust, ver int
		var created, updated time.Time
		var pid, qty sql.NullInt64
		if err := rows.Scan(&cid, &cust, &ver, &created, &updated, &pid, &qty); err != nil {
			return nil, err
		}
		if v == nil {
			v = &cartView{ShoppingCartID: cid, CustomerID: cust, Version: ver, Items: []cartItem{},
				CreatedAt: created.UTC().Format(time.RFC3339Nano), UpdatedAt: updated.UTC().Format(time.RFC3339Nano)}
		}
		if pid.Valid {
			v.Items = append(v.Items, cartItem{ProductID: int(pid.Int64), Quantity: int(qty.Int64)})
			v.TotalQuantity += int(qty.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if v == nil {
		return nil, errNotFound
	}
	sort.Slice(v.Items, func(i, j int) bool { return v.Items[i].ProductID < v.Items[j].ProductID })
	v.ItemCount = len(v.Items)
	return v, nil
}

// AddItem: one transaction. SELECT ... FOR UPDATE locks only this cart's row, so concurrent adds
// to the same cart take turns (no lost updates, and the 50-product limit can't be raced past),
// while adds to different carts lock different rows and don't wait for each other.
func (s *mysqlStore) AddItem(ctx context.Context, id, productID, qty int64) error {
	for attempt := 0; ; attempt++ {
		err := s.addItemTx(ctx, id, productID, qty)
		var me *mysql.MySQLError
		if errors.As(err, &me) && (me.Number == 1213 || me.Number == 1205) && attempt < 2 { // deadlock / lock wait timeout
			s.log.Warn("retrying add item", "mysql_error", me.Number)
			continue
		}
		return err
	}
}

func (s *mysqlStore) addItemTx(ctx context.Context, id, productID, qty int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var items int
	var exists bool
	err = tx.QueryRowContext(ctx, `
		SELECT (SELECT COUNT(*) FROM cart_items WHERE cart_id = c.cart_id),
		       EXISTS (SELECT 1 FROM cart_items WHERE cart_id = c.cart_id AND product_id = ?)
		FROM shopping_carts c WHERE c.cart_id = ? FOR UPDATE`, productID, id).Scan(&items, &exists)
	if errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if !exists && items >= maxItemsPerCart {
		return errCartFull
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cart_items (cart_id, product_id, quantity) VALUES (?, ?, ?) AS new
		ON DUPLICATE KEY UPDATE quantity = cart_items.quantity + new.quantity`, id, productID, qty); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE shopping_carts SET version = version + 1, updated_at = CURRENT_TIMESTAMP(6) WHERE cart_id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *mysqlStore) History(ctx context.Context, customerID int64) ([]cartView, error) {
	// Served by idx_customer_created: index range scan, already in created_at order (no filesort).
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.cart_id, c.customer_id, c.version, c.created_at, c.updated_at,
		       (SELECT COUNT(*) FROM cart_items i WHERE i.cart_id = c.cart_id),
		       (SELECT COALESCE(SUM(quantity), 0) FROM cart_items i WHERE i.cart_id = c.cart_id)
		FROM shopping_carts c WHERE c.customer_id = ? ORDER BY c.created_at DESC LIMIT 100`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	carts := []cartView{}
	for rows.Next() {
		var v cartView
		var created, updated time.Time
		if err := rows.Scan(&v.ShoppingCartID, &v.CustomerID, &v.Version, &created, &updated, &v.ItemCount, &v.TotalQuantity); err != nil {
			return nil, err
		}
		v.CreatedAt, v.UpdatedAt, v.Items = created.UTC().Format(time.RFC3339Nano), updated.UTC().Format(time.RFC3339Nano), []cartItem{}
		carts = append(carts, v)
	}
	return carts, rows.Err()
}

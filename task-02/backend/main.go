package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	secretPath          = "/run/secrets/db-password"
	defaultPassword     = "root@123"
	dsnTemplate         = "root:%s@tcp(db:3306)/web-db"
	connectMaxAttempts  = 60
	connectRetryDelay   = time.Second
	healthCheckArg      = "-healthcheck"
	healthCheckURL      = "http://127.0.0.1:8000/healthz"
	healthCheckTimeout  = 2 * time.Second
)

type Item struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

var ErrDuplicateName = errors.New("item with this name already exists")
var ErrItemNotFound = errors.New("item not found")

const mysqlErrDuplicateEntry = 1062

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests handled, by method, route and status code.",
	}, []string{"method", "route", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency in seconds, by method and route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})
)

// statusRecorder wraps a ResponseWriter to capture the status code written,
// since http.ResponseWriter doesn't expose it after the fact.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// withMetrics records request count and latency for a handler under a fixed
// route label (never the raw URL), so per-item IDs can't blow up cardinality.
func withMetrics(method, route string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h(rec, r)
		httpRequestsTotal.WithLabelValues(method, route, strconv.Itoa(rec.status)).Inc()
		httpRequestDuration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	}
}

func loadPassword(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not read secret from %s, using fallback environment variable or default...\n", path)
		if v, ok := os.LookupEnv("DB_PASSWORD"); ok {
			return v
		}
		return defaultPassword
	}
	return strings.TrimSpace(string(data))
}

func connect(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 1; attempt <= connectMaxAttempts; attempt++ {
		if lastErr = db.Ping(); lastErr == nil {
			fmt.Println("Database connection established successfully!")
			return db, nil
		}
		fmt.Printf("Database connection failed. Retrying in 1 second... (%d/%d)\n", attempt, connectMaxAttempts)
		time.Sleep(connectRetryDelay)
	}
	return nil, fmt.Errorf("could not connect to database after %d attempts: %w", connectMaxAttempts, lastErr)
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS items (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(255)
	)`); err != nil {
		return err
	}
	return ensureUniqueNameIndex(ctx, db)
}

// ensureUniqueNameIndex adds a unique index on items.name if it isn't already
// present. CREATE TABLE IF NOT EXISTS is a no-op on a table that already
// exists from before this constraint was introduced, so it can't be relied
// on to retrofit the index.
func ensureUniqueNameIndex(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = 'items' AND index_name = 'uq_items_name'
	`).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	if _, err := db.ExecContext(ctx, `ALTER TABLE items ADD UNIQUE INDEX uq_items_name (name)`); err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlErrDuplicateEntry {
			fmt.Fprintln(os.Stderr, "warning: could not add unique index on items.name because duplicate names already exist; remove duplicates to enforce uniqueness")
			return nil
		}
		return err
	}
	return nil
}

func seed(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM items").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	fmt.Println("Database is empty. Seeding initial items...")
	for i := 0; i < 5; i++ {
		if _, err := db.ExecContext(ctx, "INSERT INTO items (name) VALUES (?)", fmt.Sprintf("Item #%d", i)); err != nil {
			return err
		}
	}
	fmt.Println("Database seeded successfully!")
	return nil
}

func listItems(ctx context.Context, db *sql.DB) ([]Item, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, name FROM items")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func createItem(ctx context.Context, db *sql.DB, name string) (Item, error) {
	res, err := db.ExecContext(ctx, "INSERT INTO items (name) VALUES (?)", name)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlErrDuplicateEntry {
			return Item{}, ErrDuplicateName
		}
		return Item{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Item{}, err
	}
	return Item{ID: id, Name: name}, nil
}

func deleteItem(ctx context.Context, db *sql.DB, id int64) error {
	res, err := db.ExecContext(ctx, "DELETE FROM items WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrItemNotFound
	}
	return nil
}

func handleHealthz(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			http.Error(w, "db unreachable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// runHealthCheck lets the backend binary double as its own healthcheck probe,
// since the scratch runtime image has no shell or curl to exec instead.
func runHealthCheck() {
	client := http.Client{Timeout: healthCheckTimeout}
	resp, err := client.Get(healthCheckURL)
	if err != nil {
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}

func handleList(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := listItems(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}
}

type createItemRequest struct {
	Name string `json:"name"`
}

func handleCreate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createItemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		item, err := createItem(r.Context(), db, req.Name)
		if err != nil {
			if errors.Is(err, ErrDuplicateName) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(item)
	}
}

func handleDelete(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid item id", http.StatusBadRequest)
			return
		}
		if err := deleteItem(r.Context(), db, id); err != nil {
			if errors.Is(err, ErrItemNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == healthCheckArg {
		runHealthCheck()
		return
	}

	password := loadPassword(secretPath)
	dsn := fmt.Sprintf(dsnTemplate, password)

	db, err := connect(dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := migrate(ctx, db); err != nil {
		log.Fatal(err)
	}
	if err := seed(ctx, db); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz(db))
	mux.HandleFunc("GET /api/{$}", withMetrics("GET", "/api", handleList(db)))
	mux.HandleFunc("POST /api/items", withMetrics("POST", "/api/items", handleCreate(db)))
	mux.HandleFunc("DELETE /api/items/{id}", withMetrics("DELETE", "/api/items/{id}", handleDelete(db)))
	mux.Handle("GET /metrics", promhttp.Handler())

	log.Println("Listening on :8000")
	log.Fatal(http.ListenAndServe(":8000", mux))
}

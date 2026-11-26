// drover-api records waitlist sign-ups from droverstack.com.
//
// Traefik routes droverstack.com/api/* here; everything else goes to the
// static nginx container. Sign-ups land in a private Postgres that only this
// service can reach.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const listenAddr = ":8080"

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe /api/health on the local server and exit 0 if it is healthy")
	flag.Parse()
	if *healthcheck {
		os.Exit(probe())
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("exiting", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := connect(ctx, log)
	if err != nil {
		return err
	}
	defer store.Close()

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           newHandler(store, newLimiter(5, 10*time.Minute), log),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "addr", listenAddr)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

// connect builds the DSN from the same POSTGRES_* variables the database
// container is configured with, waits for the database, and applies the schema.
func connect(ctx context.Context, log *slog.Logger) (*pgStore, error) {
	host := envOr("POSTGRES_HOST", "drover-db")
	user, pass, db := os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD"), os.Getenv("POSTGRES_DB")
	if user == "" || pass == "" || db == "" {
		return nil, errors.New("POSTGRES_USER, POSTGRES_PASSWORD and POSTGRES_DB must be set")
	}
	dsn := (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, pass),
		Host:     net.JoinHostPort(host, "5432"),
		Path:     "/" + db,
		RawQuery: "sslmode=disable", // private Docker network only
	}).String()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 4

	var pool *pgxpool.Pool
	for attempt := 1; ; attempt++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			err = pool.Ping(ctx)
		}
		if err == nil {
			break
		}
		if pool != nil {
			pool.Close()
		}
		if attempt == 30 || ctx.Err() != nil {
			return nil, fmt.Errorf("database unreachable: %w", err)
		}
		log.Warn("waiting for database", "attempt", attempt, "err", err)
		time.Sleep(2 * time.Second)
	}

	s := &pgStore{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func probe() int {
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://127.0.0.1" + listenAddr + "/api/health")
	if err != nil {
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

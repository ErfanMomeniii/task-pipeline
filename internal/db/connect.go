package db

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/erfanmomeniii/task-pipeline/migrations"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect creates a pgx connection pool and returns it along with sqlc Queries.
func Connect(ctx context.Context, dsn string, log *slog.Logger) (*pgxpool.Pool, *Queries, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("pgxpool.New: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("pool.Ping: %w", err)
	}

	log.Info("connected to database")
	return pool, New(pool), nil
}

// Migrate runs all pending up migrations.
func Migrate(dsn string, log *slog.Logger) error {
	d, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("iofs.New: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, dsn)
	if err != nil {
		return fmt.Errorf("migrate.New: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate.Up: %w", err)
	}

	log.Info("database migrations applied")
	return nil
}

// DSN builds a PostgreSQL connection string from components.
// Uses net/url to safely escape special characters in credentials.
func DSN(host string, port int, user, password, dbname, sslmode string) string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     fmt.Sprintf("%s:%d", host, port),
		Path:     dbname,
		RawQuery: fmt.Sprintf("sslmode=%s", url.QueryEscape(sslmode)),
	}
	return u.String()
}

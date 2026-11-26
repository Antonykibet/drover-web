package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is what the handler needs from the database.
type Store interface {
	// Add records a sign-up. A repeat of an address already on the list
	// (compared case-insensitively) is not an error and changes nothing.
	Add(ctx context.Context, e Entry) error
	Ping(ctx context.Context) error
}

type Entry struct {
	Email string
	Size  string
	Tool  string
}

type pgStore struct{ pool *pgxpool.Pool }

const schema = `
CREATE TABLE IF NOT EXISTS waitlist (
	id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	email      text        NOT NULL,
	size       text        NOT NULL,
	tool       text        NOT NULL DEFAULT '',
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS waitlist_email_key ON waitlist (lower(email));
`

func (s *pgStore) migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schema)
	return err
}

func (s *pgStore) Add(ctx context.Context, e Entry) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO waitlist (email, size, tool) VALUES ($1, $2, $3)
		 ON CONFLICT ((lower(email))) DO NOTHING`,
		e.Email, e.Size, e.Tool)
	return err
}

func (s *pgStore) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *pgStore) Close() { s.pool.Close() }

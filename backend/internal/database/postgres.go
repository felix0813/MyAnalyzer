package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Client struct {
	pool *pgxpool.Pool
}

func New(databaseURL string) (*Client, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("database URL is required")
	}

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	return &Client{pool: pool}, nil
}

func (c *Client) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	c.pool.Close()
	return nil
}

func (c *Client) Query(ctx context.Context, sql string, args ...any) ([]byte, error) {
	if c == nil || c.pool == nil {
		return nil, errors.New("database client is not initialized")
	}
	var value []byte
	if err := c.pool.QueryRow(ctx, sql, args...).Scan(&value); err != nil {
		return nil, fmt.Errorf("postgres query failed: %w", err)
	}
	return value, nil
}

func (c *Client) QueryInt(ctx context.Context, sql string, args ...any) (int, error) {
	if c == nil || c.pool == nil {
		return 0, errors.New("database client is not initialized")
	}
	var value int
	if err := c.pool.QueryRow(ctx, sql, args...).Scan(&value); err != nil {
		return 0, fmt.Errorf("postgres query failed: %w", err)
	}
	return value, nil
}

func (c *Client) Exec(ctx context.Context, sql string, args ...any) error {
	if c == nil || c.pool == nil {
		return errors.New("database client is not initialized")
	}
	if _, err := c.pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("postgres exec failed: %w", err)
	}
	return nil
}

func (c *Client) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	if c == nil || c.pool == nil {
		return errors.New("database client is not initialized")
	}
	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.pool == nil {
		return errors.New("database client is not initialized")
	}
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := c.pool.Ping(pingCtx); err != nil {
		return fmt.Errorf("postgres ping failed: %w", err)
	}
	return nil
}

func ReadSQLFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read SQL file %q: %w", path, err)
	}
	return string(content), nil
}

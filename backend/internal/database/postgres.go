package database

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Client struct {
	DatabaseURL string
}

func New(databaseURL string) *Client {
	return &Client{DatabaseURL: databaseURL}
}

func (c *Client) Query(ctx context.Context, sql string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "psql", c.DatabaseURL, "-X", "-q", "-t", "-A", "-c", sql)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("psql query failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return bytes.TrimSpace(stdout.Bytes()), nil
}

func (c *Client) Exec(ctx context.Context, sql string) error {
	_, err := c.Query(ctx, sql)
	return err
}

func (c *Client) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.Exec(pingCtx, "SELECT 1;")
}

func Quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

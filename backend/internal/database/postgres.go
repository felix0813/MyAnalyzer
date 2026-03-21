package database

/*
#cgo CFLAGS: -I/usr/include/postgresql
#cgo LDFLAGS: -lpq
#include <libpq-fe.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"
)

type Client struct {
	DatabaseURL string
}

func New(databaseURL string) (*Client, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("database URL is required")
	}
	return &Client{DatabaseURL: databaseURL}, nil
}

func (c *Client) Close() error {
	return nil
}

func (c *Client) Query(ctx context.Context, sql string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer C.PQfinish(conn)

	result, err := execSQL(conn, sql)
	if err != nil {
		return nil, err
	}
	defer C.PQclear(result)

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if status := C.PQresultStatus(result); status != C.PGRES_TUPLES_OK {
		return nil, fmt.Errorf("postgres query failed: %s", strings.TrimSpace(C.GoString(C.PQresultErrorMessage(result))))
	}

	rows := int(C.PQntuples(result))
	cols := int(C.PQnfields(result))
	if rows != 1 || cols != 1 {
		return nil, fmt.Errorf("expected single-row single-column result, got %d rows and %d columns", rows, cols)
	}
	if C.PQgetisnull(result, 0, 0) != 0 {
		return nil, nil
	}

	length := int(C.PQgetlength(result, 0, 0))
	value := C.PQgetvalue(result, 0, 0)
	return []byte(C.GoStringN(value, C.int(length))), nil
}

func (c *Client) Exec(ctx context.Context, sql string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	conn, err := c.connect()
	if err != nil {
		return err
	}
	defer C.PQfinish(conn)

	result, err := execSQL(conn, sql)
	if err != nil {
		return err
	}
	defer C.PQclear(result)

	if err := ctx.Err(); err != nil {
		return err
	}

	status := C.PQresultStatus(result)
	if status != C.PGRES_COMMAND_OK && status != C.PGRES_TUPLES_OK {
		return fmt.Errorf("postgres exec failed: %s", strings.TrimSpace(C.GoString(C.PQresultErrorMessage(result))))
	}
	return nil
}

func (c *Client) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.Exec(pingCtx, "SELECT 1;")
}

func Quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func ReadSQLFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read SQL file %q: %w", path, err)
	}
	return string(content), nil
}

func (c *Client) connect() (*C.PGconn, error) {
	connStr := C.CString(c.DatabaseURL)
	defer C.free(unsafe.Pointer(connStr))

	conn := C.PQconnectdb(connStr)
	if conn == nil {
		return nil, errors.New("postgres connect failed: nil connection")
	}
	if C.PQstatus(conn) != C.CONNECTION_OK {
		err := strings.TrimSpace(C.GoString(C.PQerrorMessage(conn)))
		C.PQfinish(conn)
		return nil, fmt.Errorf("postgres connect failed: %s", err)
	}
	return conn, nil
}

func execSQL(conn *C.PGconn, sql string) (*C.PGresult, error) {
	query := C.CString(sql)
	defer C.free(unsafe.Pointer(query))

	result := C.PQexec(conn, query)
	if result == nil {
		return nil, fmt.Errorf("postgres exec failed: %s", strings.TrimSpace(C.GoString(C.PQerrorMessage(conn))))
	}
	return result, nil
}

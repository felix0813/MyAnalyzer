package database

import (
	"context"
	"testing"
)

func TestNewRejectsEmptyDatabaseURL(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("expected empty database URL error")
	}
}

func TestClientCloseNilSafe(t *testing.T) {
	var client *Client
	if err := client.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
}

func TestReadSQLFileMissing(t *testing.T) {
	if _, err := ReadSQLFile("missing.sql"); err == nil {
		t.Fatal("expected read error")
	}
}

func TestPingNilPoolPanicsGuardedByConstructor(t *testing.T) {
	client := &Client{}
	if err := client.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error with nil pool")
	}
}

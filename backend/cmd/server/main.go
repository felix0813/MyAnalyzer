package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"myanalyzer/backend/internal/config"
	"myanalyzer/backend/internal/database"
	"myanalyzer/backend/internal/history"
)

func main() {
	cfg := config.Load()
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database setup failed: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("database close error: %v", err)
		}
	}()

	if err := db.Ping(context.Background()); err != nil {
		log.Fatalf("database ping failed: %v", err)
	}

	handler := history.NewHandler(db)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("history backend listening on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

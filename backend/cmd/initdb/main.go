package main

import (
	"context"
	"flag"
	"log"

	"myanalyzer/backend/internal/config"
	"myanalyzer/backend/internal/database"
)

func main() {
	cfg := config.Load()

	sqlFile := flag.String("file", "init.sql", "path to SQL initialization file")
	flag.Parse()

	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database setup failed: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("database close error: %v", err)
		}
	}()

	content, err := database.ReadSQLFile(*sqlFile)
	if err != nil {
		log.Fatalf("load init SQL failed: %v", err)
	}

	if err := db.Exec(context.Background(), content); err != nil {
		log.Fatalf("run init SQL failed: %v", err)
	}

	log.Printf("database initialized with %s", *sqlFile)
}

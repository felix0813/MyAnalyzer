package config

import "os"

type Config struct {
	ListenAddr  string
	DatabaseURL string
}

func Load() Config {
	return Config{
		ListenAddr:  getEnv("LISTEN_ADDR", ":8000"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@127.0.0.1:5432/myanalyzer?sslmode=disable"),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

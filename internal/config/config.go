package config

import (
	"log/slog"
	"os"
)

const (
	defaultAddr     = ":8080"
	defaultLogLevel = slog.LevelInfo
)

type Config struct {
	Addr     string
	LogLevel slog.Level
}

func Load() Config {
	return Config{
		Addr:     envOrDefault("STORYFORGE_ADDR", defaultAddr),
		LogLevel: parseLogLevel(envOrDefault("STORYFORGE_LOG_LEVEL", defaultLogLevel.String())),
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func parseLogLevel(raw string) slog.Level {
	var level slog.Level
	if err := level.UnmarshalText([]byte(raw)); err != nil {
		return defaultLogLevel
	}

	return level
}

// internal/logger/logger.go
package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/StevenC4/docker-coredns-sync/internal/config"
)

// SetupLogger configures zerolog based on the settings, including caller info and additional metadata.
func SetupLogger(cfg *config.LoggingConfig) zerolog.Logger {
	// Use a ConsoleWriter for human-friendly output in development.
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02 15:04:05",
	}

	// Parse the log level from configuration.
	levelStr := strings.ToLower(cfg.Level)
	level, err := zerolog.ParseLevel(levelStr)
	if err != nil {
		level = zerolog.InfoLevel
	}

	// Set global log level.
	zerolog.SetGlobalLevel(level)

	// Optionally, set a custom time format.
	zerolog.TimeFieldFormat = time.RFC3339

	// Retrieve hostname for additional context.
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}

	// Build the logger with caller information and extra metadata.
	logger := zerolog.New(consoleWriter).
		With().
		Timestamp().
		Caller(). // Adds file and line number info to each log.
		Str("service", "docker_coredns_sync").
		// Str("env", cfg.Environment). // Assuming your Settings has an Environment field.
		Str("host", hostname).
		Logger()

	return logger
}

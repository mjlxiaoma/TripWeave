// Package logger provides the structured logger.
package logger

import (
	"log/slog"
	"os"
)

// New creates a JSON logger writing to stdout.
func New() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

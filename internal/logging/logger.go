package logging

import (
	"io"
	"log/slog"
	"os"
)

func NewLogger(level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(io.MultiWriter(os.Stdout, captureWriter{}), &slog.HandlerOptions{Level: level})
	return slog.New(handler)
}

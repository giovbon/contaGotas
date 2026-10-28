package platform

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// ParseLogLevel traduz LOG_LEVEL para slog.Level.
func ParseLogLevel(bruto string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(bruto)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("platform: LOG_LEVEL inválido: %q (use debug, info, warn ou error)", bruto)
	}
}

// NewLogger devolve o logger de processo, escrevendo texto em stderr.
func NewLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

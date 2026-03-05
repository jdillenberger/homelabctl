package logging

import (
	"log/slog"
	"os"
)

// Setup initializes the slog default logger based on CLI flags.
// If jsonOutput is true, a JSON handler is used; otherwise a text handler.
func Setup(verbose, quiet, jsonOutput bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	if quiet {
		level = slog.LevelWarn
	}

	var handler slog.Handler
	if jsonOutput {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}

	slog.SetDefault(slog.New(handler))
}

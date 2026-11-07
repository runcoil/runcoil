package log

import (
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/spf13/pflag"
)

// Config is the logging configuration.
type Config struct {
	Level  slog.Level
	Format string
}

// RegisterFlags declares every key with its default and help text.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.TextVar(&c.Level, "log.level", slog.LevelInfo, "Lowest level written: debug, info, warn or error.")
	fs.StringVar(&c.Format, "log.format", "text", "Output format: text for a terminal, json for production.")
}

// Validate checks the logging configuration.
func (c *Config) Validate() error {
	if c.Format != "text" && c.Format != "json" {
		return errors.New("format must be text or json")
	}

	return nil
}

// NewLogger creates the logger from the configuration and makes it the slog default.
func NewLogger(cfg Config) *slog.Logger {
	var handler slog.Handler

	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.Level})
	} else {
		handler = tint.NewTextHandler(os.Stdout, &tint.Options{
			Level:      cfg.Level,
			TimeFormat: time.RFC3339,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Value.Kind() == slog.KindAny {
					if _, ok := a.Value.Any().(error); ok {
						return tint.Attr(9, a)
					}
				}

				return a
			},
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}

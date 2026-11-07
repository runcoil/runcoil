package log

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/lmittmann/tint"
	"github.com/spf13/pflag"
)

// Config is the logging configuration.
type Config struct {
	Level  slog.Level
	Format Format
}

// RegisterFlags declares every key with its default and help text.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.TextVar(&c.Level, "log.level", slog.LevelInfo, "Lowest level written: debug, info, warn or error.")
	fs.TextVar(&c.Format, "log.format", FormatText, "Output format: text for a terminal, json for production.")
}

// Validate returns every invalid value at once.
func (c *Config) Validate() error {
	if c.Format != FormatJSON && c.Format != FormatText {
		return errors.New("log.format must be text or json")
	}

	return nil
}

// Format is the output format of the logger.
type Format uint8

const (
	// FormatJSON writes one JSON object per line.
	FormatJSON Format = iota
	// FormatText writes colored text for a terminal.
	FormatText
)

// String returns the name of the format.
func (f Format) String() string {
	switch f {
	case FormatJSON:
		return "json"
	case FormatText:
		return "text"
	default:
		return fmt.Sprintf("Format(%d)", uint8(f))
	}
}

// UnmarshalText implements [encoding.TextUnmarshaler].
func (f *Format) UnmarshalText(text []byte) error {
	switch strings.ToLower(string(text)) {
	case "json":
		*f = FormatJSON
	case "text":
		*f = FormatText
	default:
		return fmt.Errorf("unknown log format %q, want text or json", text)
	}

	return nil
}

// MarshalText implements [encoding.TextMarshaler].
func (f Format) MarshalText() ([]byte, error) {
	return []byte(f.String()), nil
}

// NewLogger creates the logger from the configuration and makes it the slog default.
func NewLogger(cfg Config) *slog.Logger {
	var handler slog.Handler

	if cfg.Format == FormatJSON {
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

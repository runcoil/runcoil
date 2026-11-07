package main

import (
	"fmt"

	"github.com/spf13/pflag"

	"github.com/runcoil/runcoil/internal/engine"
	"github.com/runcoil/runcoil/internal/log"
	"github.com/runcoil/runcoil/internal/postgres"
)

// Config is the configuration of the server.
type Config struct {
	Log      log.Config
	Postgres postgres.Config
	Engine   engine.Config
}

// RegisterFlags registers the flags of every section.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	c.Log.RegisterFlags(fs)
	c.Postgres.RegisterFlags(fs)
	c.Engine.RegisterFlags(fs)
}

// Validate checks every section of the configuration.
func (c *Config) Validate() error {
	if err := c.Log.Validate(); err != nil {
		return fmt.Errorf("log: %w", err)
	}

	if err := c.Postgres.Validate(); err != nil {
		return fmt.Errorf("postgres: %w", err)
	}

	if err := c.Engine.Validate(); err != nil {
		return fmt.Errorf("engine: %w", err)
	}

	return nil
}

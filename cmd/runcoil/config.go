package main

import (
	"fmt"

	"github.com/spf13/pflag"

	"github.com/runcoil/runcoil/internal/http"
	"github.com/runcoil/runcoil/internal/log"
	"github.com/runcoil/runcoil/internal/storage/postgres"
)

// Config is the configuration of the server.
type Config struct {
	Log         log.Config
	Postgres    postgres.Config
	AutoMigrate bool
	HTTP        http.Config
}

// RegisterFlags registers the flags of every section.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	c.Log.RegisterFlags(fs)
	c.Postgres.RegisterFlags(fs)
	fs.BoolVar(&c.AutoMigrate, "postgres.auto-migrate", false, "Apply the pending migrations at boot.")
	c.HTTP.RegisterFlags(fs)
}

// Validate checks every section of the configuration.
func (c *Config) Validate() error {
	if err := c.Log.Validate(); err != nil {
		return fmt.Errorf("log: %w", err)
	}

	if err := c.Postgres.Validate(); err != nil {
		return fmt.Errorf("postgres: %w", err)
	}

	if err := c.HTTP.Validate(); err != nil {
		return fmt.Errorf("http: %w", err)
	}

	return nil
}

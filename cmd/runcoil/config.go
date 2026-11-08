package main

import (
	"fmt"

	"github.com/spf13/pflag"

	"github.com/runcoil/runcoil/internal/bootstrap"
	"github.com/runcoil/runcoil/internal/dispatcher"
	"github.com/runcoil/runcoil/internal/http"
	"github.com/runcoil/runcoil/internal/log"
	"github.com/runcoil/runcoil/internal/storage/postgres"
)

// Config is the configuration of the server.
type Config struct {
	AutoMigrate bool
	Log         log.Config
	Postgres    postgres.Config
	HTTP        http.Config
	Engine      dispatcher.EngineConfig
	Worker      dispatcher.WorkerConfig
	Bootstrap   bootstrap.Config
}

// RegisterFlags registers the flags of every section.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&c.AutoMigrate, "auto-migrate", false, "Apply the pending migrations at boot.")
	c.Log.RegisterFlags(fs)
	c.Postgres.RegisterFlags(fs)
	c.HTTP.RegisterFlags(fs)
	c.Engine.RegisterFlags(fs)
	c.Worker.RegisterFlags(fs)
	c.Bootstrap.RegisterFlags(fs)
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

	if err := c.Engine.Validate(); err != nil {
		return fmt.Errorf("engine: %w", err)
	}

	if err := c.Worker.Validate(); err != nil {
		return fmt.Errorf("worker: %w", err)
	}

	if err := c.Bootstrap.Validate(); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	return nil
}

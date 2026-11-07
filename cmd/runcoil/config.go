package main

import (
	"errors"

	"github.com/spf13/pflag"

	"github.com/runcoil/runcoil/internal/log"
	"github.com/runcoil/runcoil/internal/postgres"
	"github.com/runcoil/runcoil/internal/server"
)

// Config gathers the configs of the server's packages.
type Config struct {
	HTTP     server.Config
	Postgres postgres.Config
	Log      log.Config
}

// RegisterFlags registers every package's flags.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	c.HTTP.RegisterFlags(fs)
	c.Postgres.RegisterFlags(fs)
	c.Log.RegisterFlags(fs)
}

// Validate checks every package and returns every error at once.
func (c *Config) Validate() error {
	return errors.Join(
		c.HTTP.Validate(),
		c.Postgres.Validate(),
		c.Log.Validate(),
	)
}

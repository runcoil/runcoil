package account

import (
	"errors"
	"time"

	"github.com/spf13/pflag"
)

const minSessionMaxInactive = 5 * time.Minute

// Config is the configuration of the accounts and their sessions.
type Config struct {
	SessionMaxLifetime time.Duration
	SessionMaxInactive time.Duration

	// BootstrapAdminUsername and BootstrapAdminPassword are the seed's bootstrap admin,
	// which main copies here: a login as that user with that password is asked to change
	// it. They have no key of their own.
	BootstrapAdminUsername string
	BootstrapAdminPassword string
}

// RegisterFlags declares every key with its default and help text.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&c.SessionMaxLifetime, "session.max-lifetime", 30*24*time.Hour, "Longest time a dashboard session lasts.")
	fs.DurationVar(&c.SessionMaxInactive, "session.max-inactive", 7*24*time.Hour, "Time without activity after which a dashboard session ends.")
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	if c.SessionMaxLifetime <= 0 {
		return errors.New("max lifetime must be positive")
	}

	// A session records its activity at most once a minute, so a shorter limit would end
	// sessions in use.
	if c.SessionMaxInactive < minSessionMaxInactive {
		return errors.New("max inactive must be at least 5m")
	}

	return nil
}

package bootstrap

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/pflag"

	"github.com/runcoil/runcoil/internal/domain"
)

// ProjectName is the project the first start creates. The first phases run every Run there.
const ProjectName = "default"

// apiKeyPrefix starts every API key, so a secret scanner can spot a leaked one.
const apiKeyPrefix = "rc_api_"

// Config is the first-start configuration.
type Config struct {
	APIKey string
}

// RegisterFlags declares every key with its default and help text.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.APIKey, "bootstrap.api-key", "", "API key added to the default project at boot when it is not stored yet.")
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return nil
	}

	if !strings.HasPrefix(c.APIKey, apiKeyPrefix) || len(c.APIKey) == len(apiKeyPrefix) {
		return errors.New("api key must start with " + apiKeyPrefix)
	}

	return nil
}

// Store runs the first start in one transaction.
type Store interface {
	Bootstrap(ctx context.Context, projectID uuid.UUID, projectName string, key *domain.BootstrapKey) (projectCreated, keyCreated bool, err error)
}

// Run creates the default project when the database has none, and adds the configured
// API key when it is not stored yet. It runs at every boot, so a database made before
// the key was set still gets it.
func Run(ctx context.Context, cfg Config, logger *slog.Logger, store Store) error {
	projectID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("new project id: %w", err)
	}

	var key *domain.BootstrapKey
	if cfg.APIKey != "" {
		keyID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("new api key id: %w", err)
		}

		hash := sha256.Sum256([]byte(cfg.APIKey))
		key = &domain.BootstrapKey{ID: keyID, Hash: hash[:]}
	}

	projectCreated, keyCreated, err := store.Bootstrap(ctx, projectID, ProjectName, key)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	if projectCreated {
		logger.InfoContext(ctx, "project created", slog.String("name", ProjectName))
	}

	if keyCreated {
		logger.InfoContext(ctx, "bootstrap api key added", slog.String("project", ProjectName))
	}

	return nil
}

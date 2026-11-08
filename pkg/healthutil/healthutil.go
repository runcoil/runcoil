package healthutil

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Check returns an error when the part of the process it checks cannot take work.
type Check func(ctx context.Context) error

// Checks are the readiness checks of a process. The server that serves /readyz runs
// them, so each part of the process reports through the one port the probes call.
type Checks struct {
	mu     sync.RWMutex
	checks []namedCheck
}

type namedCheck struct {
	name  string
	check Check
}

// New returns an empty set of checks.
func New() *Checks {
	return &Checks{}
}

// Add registers a check under name.
func (c *Checks) Add(name string, check Check) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.checks = append(c.checks, namedCheck{name: name, check: check})
}

// Ready runs every check in the order they were added and returns their failures, each
// prefixed with the name of its check.
func (c *Checks) Ready(ctx context.Context) error {
	c.mu.RLock()
	checks := c.checks
	c.mu.RUnlock()

	var errs []error
	for _, nc := range checks {
		if err := nc.check(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", nc.name, err))
		}
	}

	return errors.Join(errs...)
}

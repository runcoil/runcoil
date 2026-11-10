package postgres

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/runcoil/runcoil/internal/domain"
)

// retryPolicyJSON is how tasks.retry_policy and the retry of runs.config store a policy.
// Delays are Go duration strings, such as "1m30s".
type retryPolicyJSON struct {
	MaxAttempts int      `json:"max_attempts,omitempty"`
	Delays      []string `json:"delays,omitempty"`
}

// runConfigJSON is the part of runs.config that the engine reads: the Task's settings when
// the Run was created.
type runConfigJSON struct {
	Retry *retryPolicyJSON `json:"retry"`
}

func retryPolicyToJSON(p domain.RetryPolicy) retryPolicyJSON {
	policy := retryPolicyJSON{MaxAttempts: p.MaxAttempts}
	for _, d := range p.Delays {
		policy.Delays = append(policy.Delays, d.String())
	}

	return policy
}

func retryPolicyFromJSON(p retryPolicyJSON) (domain.RetryPolicy, error) {
	policy := domain.RetryPolicy{MaxAttempts: p.MaxAttempts}
	for _, s := range p.Delays {
		d, err := time.ParseDuration(s)
		if err != nil {
			return domain.RetryPolicy{}, fmt.Errorf("parse delay %q: %w", s, err)
		}

		policy.Delays = append(policy.Delays, d)
	}

	return policy, nil
}

// runRetryPolicy returns the retry policy in a Run's config, or nil when it has none.
func runRetryPolicy(config json.RawMessage) (*domain.RetryPolicy, error) {
	if len(config) == 0 {
		return nil, nil //nolint:nilnil // No policy is not an error.
	}

	var c runConfigJSON
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("decode run config: %w", err)
	}

	if c.Retry == nil {
		return nil, nil //nolint:nilnil // No policy is not an error.
	}

	policy, err := retryPolicyFromJSON(*c.Retry)
	if err != nil {
		return nil, fmt.Errorf("decode retry policy: %w", err)
	}

	return &policy, nil
}

package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStepRetryValidate(t *testing.T) {
	second, negative := time.Second, -time.Second

	assert.NoError(t, StepRetry{}.Validate())
	assert.NoError(t, StepRetry{Policy: &RetryPolicy{MaxAttempts: 3}, After: &second}.Validate())
	assert.EqualError(t, StepRetry{Policy: &RetryPolicy{MaxAttempts: 31}}.Validate(), "retry policy: max attempts must be between 0 and 30")
	assert.EqualError(t, StepRetry{After: &negative}.Validate(), "retry after must not be negative")
}

func TestValidateStepID(t *testing.T) {
	assert.NoError(t, ValidateStepID("charge"))
	assert.NoError(t, ValidateStepID(strings.Repeat("x", MaxStepIDBytes)))
	assert.EqualError(t, ValidateStepID(""), "step id must be 1 to 255 bytes long")
	assert.EqualError(t, ValidateStepID(strings.Repeat("x", MaxStepIDBytes+1)), "step id must be 1 to 255 bytes long")
	assert.EqualError(t, ValidateStepID("bad\x00id"), "step id must not contain a NUL byte")
}

package account

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfigValidate(t *testing.T) {
	assert.NoError(t, testConfig.Validate())
	assert.NoError(t, (&Config{SessionMaxLifetime: time.Hour, SessionMaxInactive: 5 * time.Minute}).Validate())
	assert.EqualError(t, (&Config{SessionMaxInactive: time.Hour}).Validate(), "max lifetime must be positive")
	assert.EqualError(t, (&Config{SessionMaxLifetime: time.Hour}).Validate(), "max inactive must be at least 5m")
	assert.EqualError(t, (&Config{SessionMaxLifetime: time.Hour, SessionMaxInactive: 30 * time.Second}).Validate(), "max inactive must be at least 5m")
}

package dispatcher

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

func TestRegistrationError(t *testing.T) {
	err := registrationError(&domain.RegistrationError{Message: "task is declared twice", TaskName: "a"})

	var apiErr *apierr.Error
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, apierr.CodeInvalidRegistration, apiErr.Code)
	assert.Equal(t, "task is declared twice", apiErr.Message)
	assert.Equal(t, "a", apiErr.Details["task_name"])
}

func TestNewSessionTokenHashesTheToken(t *testing.T) {
	first, firstHash, err := newSessionToken()
	require.NoError(t, err)

	second, _, err := newSessionToken()
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
	assert.Len(t, firstHash, 32)
}

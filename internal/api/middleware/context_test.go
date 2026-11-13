package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdentityFromContextWithoutAnIdentity(t *testing.T) {
	_, ok := IdentityFromContext(context.Background())

	assert.False(t, ok)
}

func TestRecordErrorOutsideARequestDoesNothing(t *testing.T) {
	assert.NotPanics(t, func() {
		RecordError(context.Background(), errors.New("x"))
	})
}

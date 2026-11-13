package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCallerFromContextWithoutACaller(t *testing.T) {
	_, ok := CallerFromContext(context.Background())

	assert.False(t, ok)
}

func TestRecordErrorOutsideARequestDoesNothing(t *testing.T) {
	assert.NotPanics(t, func() {
		RecordError(context.Background(), errors.New("x"))
	})
}

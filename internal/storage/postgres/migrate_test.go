package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrations(t *testing.T) {
	ctx := t.Context()

	m, err := NewMigrator(newTestPool(t))
	require.NoError(t, err)

	pending, err := m.HasPending(ctx)
	require.NoError(t, err)
	assert.True(t, pending, "a new database lacks every migration")

	require.NoError(t, m.Up(ctx))

	pending, err = m.HasPending(ctx)
	require.NoError(t, err)
	assert.False(t, pending, "up applies every migration")

	require.NoError(t, m.Up(ctx), "a second up does nothing")

	_, err = m.provider.DownTo(ctx, 0)
	require.NoError(t, err, "every migration rolls back")

	pending, err = m.HasPending(ctx)
	require.NoError(t, err)
	assert.True(t, pending)

	require.NoError(t, m.Up(ctx), "up works again after a full rollback")
}

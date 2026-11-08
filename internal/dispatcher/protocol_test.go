package dispatcher

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

func TestValidateRegistration(t *testing.T) {
	task := func(name string) domain.TaskDefinition { return domain.TaskDefinition{Name: name} }

	tests := []struct {
		name     string
		reg      domain.Registration
		want     string
		taskName string
	}{
		{name: "valid", reg: domain.Registration{App: "shop", Slots: 1, Tasks: []domain.TaskDefinition{task("a")}}},
		{name: "no slots", reg: domain.Registration{App: "shop", Tasks: []domain.TaskDefinition{task("a")}}, want: "slots must be at least 1"},
		{name: "no tasks", reg: domain.Registration{App: "shop", Slots: 1}, want: "a registration needs at least one task"},
		{
			name:     "task twice",
			reg:      domain.Registration{App: "shop", Slots: 1, Tasks: []domain.TaskDefinition{task("a"), task("a")}},
			want:     "task is declared twice",
			taskName: "a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRegistration(tt.reg)
			if tt.want == "" {
				assert.NoError(t, err)
				return
			}

			var apiErr *apierr.Error
			require.True(t, errors.As(err, &apiErr))
			assert.Equal(t, apierr.CodeInvalidRegistration, apiErr.Code)
			assert.Equal(t, tt.want, apiErr.Message)
			assert.Equal(t, tt.taskName, apiErr.Details["task_name"])
		})
	}
}

func TestNewSessionTokenHashesTheToken(t *testing.T) {
	first, firstHash, err := newSessionToken()
	require.NoError(t, err)

	second, _, err := newSessionToken()
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
	assert.Len(t, firstHash, 32)
}

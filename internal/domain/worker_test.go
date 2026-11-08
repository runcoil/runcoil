package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistrationValidate(t *testing.T) {
	task := func(name string) TaskDefinition { return TaskDefinition{Name: name} }

	tests := []struct {
		name string
		reg  Registration
		want *RegistrationError
	}{
		{name: "valid", reg: Registration{App: "shop", Slots: 1, Tasks: []TaskDefinition{task("a")}}},
		{name: "no slots", reg: Registration{App: "shop", Tasks: []TaskDefinition{task("a")}}, want: &RegistrationError{Message: "slots must be between 1 and 100"}},
		{name: "most slots", reg: Registration{App: "shop", Slots: MaxSlots, Tasks: []TaskDefinition{task("a")}}},
		{name: "too many slots", reg: Registration{App: "shop", Slots: MaxSlots + 1, Tasks: []TaskDefinition{task("a")}}, want: &RegistrationError{Message: "slots must be between 1 and 100"}},
		{name: "no tasks", reg: Registration{App: "shop", Slots: 1}, want: &RegistrationError{Message: "a registration needs at least one task"}},
		{
			name: "task twice",
			reg:  Registration{App: "shop", Slots: 1, Tasks: []TaskDefinition{task("a"), task("a")}},
			want: &RegistrationError{Message: "task is declared twice", TaskName: "a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.reg.Validate()
			if tt.want == nil {
				assert.NoError(t, err)
				return
			}

			regErr, ok := AsRegistrationError(err)
			require.True(t, ok)
			assert.Equal(t, tt.want, regErr)
		})
	}
}

func TestNewWorkerSession(t *testing.T) {
	reg := Registration{App: "shop", Slots: 2, Tasks: []TaskDefinition{{Name: "a"}, {Name: "b"}}}

	session, err := NewWorkerSession(reg, []byte("hash"))
	require.NoError(t, err)

	assert.NotEqual(t, [16]byte{}, [16]byte(session.ID))
	assert.Equal(t, []byte("hash"), session.TokenHash)
	assert.Equal(t, reg, session.Registration)
	require.Len(t, session.TaskIDs, 2)
	assert.NotEqual(t, session.TaskIDs[0], session.TaskIDs[1])
}

func TestNewWorkerSessionValidates(t *testing.T) {
	_, err := NewWorkerSession(Registration{App: "shop", Tasks: []TaskDefinition{{Name: "a"}}}, nil)

	regErr, ok := AsRegistrationError(err)
	require.True(t, ok)
	assert.Equal(t, "slots must be between 1 and 100", regErr.Message)
}

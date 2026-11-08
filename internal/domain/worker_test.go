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
		{name: "no slots", reg: Registration{App: "shop", Tasks: []TaskDefinition{task("a")}}, want: &RegistrationError{Message: "slots must be at least 1"}},
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

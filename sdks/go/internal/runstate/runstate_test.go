package runstate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStepID(t *testing.T) {
	s := NewState(nil)

	got := []string{s.StepID("charge"), s.StepID("charge"), s.StepID("charge#2"), s.StepID("charge"), s.StepID("refund")}

	assert.Equal(t, []string{"charge", "charge#2", "charge#2#2", "charge#3", "refund"}, got)
}

func TestLookupAndStore(t *testing.T) {
	s := NewState(map[string]Record{"charge": {Succeeded: true, Output: []byte(`1`)}})

	rec, ok := s.Lookup("charge")
	assert.True(t, ok)
	assert.Equal(t, []byte(`1`), rec.Output)

	_, ok = s.Lookup("refund")
	assert.False(t, ok)

	s.Store("refund", Record{Kind: KindError, Message: "declined"})
	rec, ok = s.Lookup("refund")
	assert.True(t, ok)
	assert.Equal(t, "declined", rec.Message)
}

func TestUnwind(t *testing.T) {
	defer func() {
		assert.True(t, IsUnwind(recover()))
	}()

	Unwind()
}

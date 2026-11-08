package healthutil

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReady(t *testing.T) {
	ok := func(context.Context) error { return nil }
	down := func(context.Context) error { return errors.New("down") }

	tests := []struct {
		name   string
		checks map[string]Check
		order  []string
		want   string
	}{
		{name: "no checks"},
		{name: "every check passes", checks: map[string]Check{"a": ok, "b": ok}, order: []string{"a", "b"}},
		{name: "one fails", checks: map[string]Check{"a": ok, "b": down}, order: []string{"a", "b"}, want: "b: down"},
		{name: "every failure", checks: map[string]Check{"a": down, "b": down}, order: []string{"a", "b"}, want: "a: down\nb: down"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()
			for _, name := range tt.order {
				c.Add(name, tt.checks[name])
			}

			err := c.Ready(context.Background())
			if tt.want == "" {
				assert.NoError(t, err)
				return
			}

			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestReadyPassesTheContext(t *testing.T) {
	type key struct{}

	c := New()
	c.Add("ctx", func(ctx context.Context) error {
		if ctx.Value(key{}) != "v" {
			return errors.New("context not passed")
		}

		return nil
	})

	assert.NoError(t, c.Ready(context.WithValue(context.Background(), key{}, "v")))
}

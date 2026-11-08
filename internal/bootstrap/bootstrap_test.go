package bootstrap

import (
	"context"
	"crypto/sha256"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/domain"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		want   string
	}{
		{name: "unset", apiKey: ""},
		{name: "api key", apiKey: "rc_api_abc"},
		{name: "no prefix", apiKey: "abc", want: "api key must start with rc_api_"},
		{name: "prefix only", apiKey: "rc_api_", want: "api key must start with rc_api_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{APIKey: tt.apiKey}
			err := cfg.Validate()
			if tt.want == "" {
				assert.NoError(t, err)
				return
			}

			assert.EqualError(t, err, tt.want)
		})
	}
}

type fakeStore struct {
	projectName string
	key         *domain.BootstrapKey
}

func (s *fakeStore) Bootstrap(_ context.Context, _ uuid.UUID, projectName string, key *domain.BootstrapKey) (bool, bool, error) {
	s.projectName = projectName
	s.key = key

	return true, key != nil, nil
}

func TestRunHashesTheKey(t *testing.T) {
	store := &fakeStore{}
	require.NoError(t, Run(context.Background(), Config{APIKey: "rc_api_abc"}, slog.New(slog.DiscardHandler), store))

	want := sha256.Sum256([]byte("rc_api_abc"))
	assert.Equal(t, ProjectName, store.projectName)
	require.NotNil(t, store.key)
	assert.Equal(t, want[:], store.key.Hash)
}

func TestRunWithoutKey(t *testing.T) {
	store := &fakeStore{}
	require.NoError(t, Run(context.Background(), Config{}, slog.New(slog.DiscardHandler), store))

	assert.Nil(t, store.key)
}

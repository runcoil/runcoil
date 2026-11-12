package account

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/argon2"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{name: "every allowed character", username: "ops.admin_1-a"},
		{name: "longest", username: strings.Repeat("a", 64)},
		{name: "empty", username: "", wantErr: true},
		{name: "too long", username: strings.Repeat("a", 65), wantErr: true},
		{name: "upper case", username: "Admin", wantErr: true},
		{name: "email", username: "admin@example.com", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}

			assert.EqualError(t, err, "must be 1 to 64 characters of a-z, 0-9, '.', '_' and '-'")
		})
	}
}

func TestHashPassword(t *testing.T) {
	hash := HashPassword("admin")

	parts := strings.Split(hash, "$")
	require.Len(t, parts, 6)
	assert.Equal(t, "argon2id", parts[1])
	assert.Equal(t, "v=19", parts[2])
	assert.Equal(t, "m=19456,t=2,p=1", parts[3])

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	require.NoError(t, err)
	assert.Len(t, salt, 16)

	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	require.NoError(t, err)
	assert.Equal(t, argon2.IDKey([]byte("admin"), salt, 2, 19456, 1, 32), key)
	assert.NotEqual(t, argon2.IDKey([]byte("Admin"), salt, 2, 19456, 1, 32), key)
	assert.NotEqual(t, hash, HashPassword("admin"))
}

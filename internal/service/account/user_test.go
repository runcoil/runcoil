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

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "every class", password: "Secret-1"},
		{name: "unicode letters", password: "Mật-khẩu1"},
		{name: "too short", password: "Sec-1ab", wantErr: true},
		{name: "no upper case", password: "secret-12", wantErr: true},
		{name: "no lower case", password: "SECRET-12", wantErr: true},
		{name: "no digit", password: "Secret-ab", wantErr: true},
		{name: "no special", password: "Secret123", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}

			assert.EqualError(t, err, "must be at least 8 characters with an upper-case letter, a lower-case letter, a digit and a special character")
		})
	}
}

func TestVerifyPassword(t *testing.T) {
	hash := HashPassword("Secret-1")

	ok, err := VerifyPassword(hash, "Secret-1")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = VerifyPassword(hash, "secret-1")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestVerifyPasswordWithOtherParameters(t *testing.T) {
	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte("admin"), salt, 3, 8*1024, 2, 16)
	hash := "$argon2id$v=19$m=8192,t=3,p=2$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key)

	ok, err := VerifyPassword(hash, "admin")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestVerifyPasswordMalformed(t *testing.T) {
	for _, hash := range []string{
		"",
		"plain",
		"$bcrypt$v=19$m=19456,t=2,p=1$c2FsdA$a2V5",
		"$argon2id$v=18$m=19456,t=2,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=x,t=2,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=19456,t=2,p=1$!!$a2V5",
		"$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$",
	} {
		_, err := VerifyPassword(hash, "admin")
		assert.ErrorIs(t, err, errMalformedHash, hash)
	}
}

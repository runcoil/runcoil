package account

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemoryKiB = 19 * 1024
	argonPasses    = 2
	argonThreads   = 1
	argonSaltLen   = 16
	argonKeyLen    = 32
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

// ValidateUsername checks that name is a valid username.
func ValidateUsername(name string) error {
	if !usernamePattern.MatchString(name) {
		return errors.New("must be 1 to 64 characters of a-z, 0-9, '.', '_' and '-'")
	}

	return nil
}

// HashPassword hashes a password with argon2id into a PHC string.
func HashPassword(password string) string {
	salt := make([]byte, argonSaltLen)
	_, _ = rand.Read(salt)

	key := argon2.IDKey([]byte(password), salt, argonPasses, argonMemoryKiB, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemoryKiB,
		argonPasses,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
}

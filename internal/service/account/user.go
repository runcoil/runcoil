package account

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemoryKiB = 19 * 1024
	argonPasses    = 2
	argonThreads   = 1
	argonSaltLen   = 16
	argonKeyLen    = 32
	maxKeyLen      = 128
)

const minPasswordLen = 8

var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

var hashSlots = make(chan struct{}, runtime.GOMAXPROCS(0))

var errMalformedHash = errors.New("malformed password hash")

// ValidateUsername checks that name is a valid username.
func ValidateUsername(name string) error {
	if !usernamePattern.MatchString(name) {
		return errors.New("must be 1 to 64 characters of a-z, 0-9, '.', '_' and '-'")
	}

	return nil
}

// ValidatePassword checks password against the password rule.
func ValidatePassword(password string) error {
	var upper, lower, digit, special bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case !unicode.IsLetter(r):
			special = true
		}
	}

	if utf8.RuneCountInString(password) < minPasswordLen || !upper || !lower || !digit || !special {
		return errors.New("must be at least 8 characters with an upper-case letter, a lower-case letter, a digit and a special character")
	}

	return nil
}

// HashPassword hashes a password with argon2id into a PHC string.
func HashPassword(password string) string {
	salt := make([]byte, argonSaltLen)
	_, _ = rand.Read(salt)

	key := idKey([]byte(password), salt, argonPasses, argonMemoryKiB, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemoryKiB,
		argonPasses,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
}

// VerifyPassword reports whether password matches hash, a PHC string from HashPassword.
func VerifyPassword(hash, password string) (bool, error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errMalformedHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errMalformedHash
	}

	var memory, passes uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &threads); err != nil {
		return false, errMalformedHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errMalformedHash
	}

	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 || len(want) > maxKeyLen {
		return false, errMalformedHash
	}

	got := idKey([]byte(password), salt, passes, memory, threads, uint32(len(want))) //nolint:gosec // At most maxKeyLen.

	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func idKey(password, salt []byte, passes, memory uint32, threads uint8, keyLen uint32) []byte {
	hashSlots <- struct{}{}
	defer func() { <-hashSlots }()

	return argon2.IDKey(password, salt, passes, memory, threads, keyLen)
}

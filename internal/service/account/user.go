package account

import (
	"errors"
	"regexp"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

// ValidateUsername checks that name is a valid username.
func ValidateUsername(name string) error {
	if !usernamePattern.MatchString(name) {
		return errors.New("must be 1 to 64 characters of a-z, 0-9, '.', '_' and '-'")
	}

	return nil
}

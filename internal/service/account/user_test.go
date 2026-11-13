package account

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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

package api

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/runcoil/runcoil/internal/apierr"
)

func TestErrorCodesMatchTheSpec(t *testing.T) {
	for code, specCode := range errorCodes {
		assert.True(t, slices.Contains(apierr.Codes, code), code)
		assert.True(t, specCode.Valid(), specCode)
		assert.Equal(t, strings.ToLower(string(code)), string(specCode))
	}
}

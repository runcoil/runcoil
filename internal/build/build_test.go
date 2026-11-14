package build

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGet(t *testing.T) {
	assert.Equal(t, Info{
		Version:   "dev",
		Commit:    "",
		BuildDate: "",
		GoVersion: runtime.Version(),
	}, Get())
}

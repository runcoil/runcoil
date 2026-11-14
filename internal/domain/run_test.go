package domain

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestRunResultEnd(t *testing.T) {
	assert.Equal(t, RunEnd{Status: RunStatusSucceeded, Output: []byte(`1`)}, RunResult{Output: []byte(`1`)}.End())

	panicked := RunResult{Failure: &Failure{Code: FailureCodePanic, Message: "boom"}}
	assert.Equal(t, RunEnd{Status: RunStatusFailed, Failure: &Failure{Code: FailureCodePanic, Message: "boom"}}, panicked.End())

	stepFailed := RunResult{Failure: &Failure{Code: FailureCodeError, Message: "smtp down", StepID: "email"}}
	assert.Equal(t, RunEnd{
		Status:  RunStatusFailed,
		Failure: &Failure{Code: FailureCodeStepFailed, Message: "smtp down", StepID: "email"},
	}, stepFailed.End())
}

func TestRunStatusEnded(t *testing.T) {
	assert.False(t, RunStatusQueued.Ended())
	assert.False(t, RunStatusRunning.Ended())
	assert.True(t, RunStatusSucceeded.Ended())
	assert.True(t, RunStatusFailed.Ended())
}

func TestFailureCodeRetriable(t *testing.T) {
	assert.True(t, FailureCodeError.Retriable())
	assert.False(t, FailureCodePanic.Retriable())
	assert.False(t, FailureCodeOutputNotEncodable.Retriable())
	assert.False(t, FailureCodeOutputTooLarge.Retriable())
}

func TestCleanMessage(t *testing.T) {
	assert.Equal(t, "smtp down", CleanMessage("smtp down"))
	assert.Equal(t, "bad\uFFFDbyte", CleanMessage("bad\x00byte"))

	long := strings.Repeat("x", MaxErrorMessageBytes+10)
	assert.Equal(t, long[:MaxErrorMessageBytes], CleanMessage(long))

	// A three-byte character across the limit is dropped whole.
	cut := CleanMessage(strings.Repeat("x", MaxErrorMessageBytes-1) + "€")
	assert.Len(t, cut, MaxErrorMessageBytes-1)
	assert.True(t, utf8.ValidString(cut))

	// NUL grows to three bytes before the cut.
	nuls := CleanMessage(strings.Repeat("\x00", MaxErrorMessageBytes))
	assert.LessOrEqual(t, len(nuls), MaxErrorMessageBytes)
	assert.True(t, utf8.ValidString(nuls))
	assert.NotContains(t, nuls, "\x00")
}

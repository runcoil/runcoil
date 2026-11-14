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

func TestFailureCleaned(t *testing.T) {
	var none *Failure
	assert.Nil(t, none.Cleaned())

	f := &Failure{Code: FailureCodeError, Message: "bad\x00byte", StepID: "email"}
	assert.Equal(t, &Failure{Code: FailureCodeError, Message: "bad\uFFFDbyte", StepID: "email"}, f.Cleaned())
	assert.Equal(t, "bad\x00byte", f.Message, "the failure itself is not changed")
}

func TestCleanMessage(t *testing.T) {
	assert.Equal(t, "smtp down", cleanMessage("smtp down"))
	assert.Equal(t, "bad\uFFFDbyte", cleanMessage("bad\x00byte"))

	long := strings.Repeat("x", MaxErrorMessageBytes+10)
	assert.Equal(t, long[:MaxErrorMessageBytes], cleanMessage(long))

	// A three-byte character across the limit is dropped whole.
	cut := cleanMessage(strings.Repeat("x", MaxErrorMessageBytes-1) + "\u20ac")
	assert.Len(t, cut, MaxErrorMessageBytes-1)
	assert.True(t, utf8.ValidString(cut))

	// NUL grows to three bytes before the cut.
	nuls := cleanMessage(strings.Repeat("\x00", MaxErrorMessageBytes))
	assert.LessOrEqual(t, len(nuls), MaxErrorMessageBytes)
	assert.True(t, utf8.ValidString(nuls))
	assert.NotContains(t, nuls, "\x00")
}

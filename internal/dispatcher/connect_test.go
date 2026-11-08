package dispatcher

import (
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/apierr"
	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
)

func TestEveryCodeHasAnErrorCode(t *testing.T) {
	for code := range errorCodes {
		assert.NotEqual(t, runcoilv1.ErrorCode_ERROR_CODE_UNSPECIFIED, errorCodes[code], code)
	}
}

func TestToConnectError(t *testing.T) {
	converted, err := toConnectError(apierr.Invalid(apierr.CodeInvalidRegistration, "task is declared twice").WithDetail("task_name", "a"))
	require.NoError(t, err)

	assert.Equal(t, connect.CodeInvalidArgument, converted.Code())

	var detail *runcoilv1.ErrorDetail
	for _, d := range converted.Details() {
		value, err := connectproto.UnmarshalErrorDetail(d)
		require.NoError(t, err)

		if msg, ok := value.(*runcoilv1.ErrorDetail); ok {
			detail = msg
		}
	}

	require.NotNil(t, detail)
	assert.Equal(t, runcoilv1.ErrorCode_ERROR_CODE_INVALID_REGISTRATION, detail.GetCode())
	assert.Equal(t, "a", detail.GetTaskName())
}

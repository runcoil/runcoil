package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/apierr"
	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
)

func TestErrorCodesMatchTheSchema(t *testing.T) {
	for _, code := range apierr.Codes {
		want, inSchema := runcoilv1.ErrorCode_value["ERROR_CODE_"+string(code)]
		got, mapped := errorCodes[code]

		assert.Equal(t, inSchema, mapped, code)

		if inSchema {
			assert.Equal(t, runcoilv1.ErrorCode(want), got, code)
		}
	}
}

func TestTranslate(t *testing.T) {
	i := errorInterceptor{logger: slog.New(slog.DiscardHandler)}
	spec := connect.Spec{Procedure: "/runcoil.v1.WorkerService/Heartbeat"}

	t.Run("an internal error hides its text", func(t *testing.T) {
		err := i.translate(context.Background(), spec, errors.New("dial tcp 10.0.0.1:5432: refused"))

		var connectErr *connect.Error
		require.True(t, errors.As(err, &connectErr))
		assert.Equal(t, connect.CodeInternal, connectErr.Code())
		assert.Equal(t, internalMessage, connectErr.Message())
	})

	t.Run("an api error keeps its code through a wrap", func(t *testing.T) {
		err := i.translate(context.Background(), spec, fmt.Errorf("complete run: %w", errStaleLease))

		var connectErr *connect.Error
		require.True(t, errors.As(err, &connectErr))
		assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
		assert.Equal(t, "lease is no longer this attempt's", connectErr.Message())
	})

	t.Run("a connect error passes through", func(t *testing.T) {
		in := connect.NewError(connect.CodeNotFound, "no such route")

		assert.Same(t, in, i.translate(context.Background(), spec, in))
	})
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

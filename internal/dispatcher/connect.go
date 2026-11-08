package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"connectrpc.com/connect/v2/connectproto"
	"connectrpc.com/validate"

	"github.com/runcoil/runcoil/internal/apierr"
	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
	"github.com/runcoil/runcoil/proto/gen/go/runcoil/v1/runcoilv1connect"
)

// Handler returns the worker protocol and the path it is served under, for the server's
// mux. The dispatcher holds the workers' streams, so it serves their calls too.
func (s *Service) Handler() (string, http.Handler) {
	server := connect.NewServer(
		errorInterceptor{logger: s.logger}.wrap,
		validate.NewServerInterceptor(),
	)
	runcoilv1connect.RegisterWorkerServiceHandler(server, workerHandler{s: s})

	mux := http.NewServeMux()
	connecthttp.Mount(mux, server)

	return "/" + runcoilv1connect.WorkerServiceName + "/", mux
}

// internalMessage is the only text a client sees for an unexpected failure.
const internalMessage = "internal error"

// errorCodes maps the API codes the worker protocol returns to the proto ErrorCode.
var errorCodes = map[apierr.Code]runcoilv1.ErrorCode{
	apierr.CodeInvalidRequest:      runcoilv1.ErrorCode_ERROR_CODE_INVALID_REQUEST,
	apierr.CodeUnauthenticated:     runcoilv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED,
	apierr.CodeInvalidRegistration: runcoilv1.ErrorCode_ERROR_CODE_INVALID_REGISTRATION,
	apierr.CodeStaleLease:          runcoilv1.ErrorCode_ERROR_CODE_STALE_LEASE,
}

// errorInterceptor is the one place that translates errors for a Connect client. A
// Connect error passes through. An apierr.Error becomes its code and detail. Any other
// error is logged, and the client gets an internal error with a fixed text.
type errorInterceptor struct {
	logger *slog.Logger
}

func (i errorInterceptor) wrap(next connect.ServerFunc) connect.ServerFunc {
	return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
		if err := next(ctx, spec, stream); err != nil {
			return i.translate(ctx, spec, err)
		}

		return nil
	}
}

func (i errorInterceptor) translate(ctx context.Context, spec connect.Spec, err error) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return connectErr
	}

	var apiErr *apierr.Error
	if errors.As(err, &apiErr) {
		converted, convErr := toConnectError(apiErr)
		if convErr != nil {
			i.logger.ErrorContext(ctx, "convert api error failed", slog.Any("error", convErr))
			return connect.NewError(connect.CodeInternal, internalMessage)
		}

		i.logger.WarnContext(ctx, "request refused",
			slog.String("procedure", spec.Procedure),
			slog.String("code", string(apiErr.Code)),
			slog.String("message", apiErr.Message),
		)

		return converted
	}

	i.logger.ErrorContext(ctx, "request failed",
		slog.String("procedure", spec.Procedure),
		slog.Any("error", err),
	)

	return connect.NewError(connect.CodeInternal, internalMessage)
}

func toConnectError(e *apierr.Error) (*connect.Error, error) {
	connectErr := connect.NewError(connectCodeOf(e.Kind), e.Message)
	if e.Code == "" {
		return connectErr, nil
	}

	detail, err := connectproto.NewErrorDetail(&runcoilv1.ErrorDetail{
		Code:     errorCodes[e.Code],
		TaskName: e.Details["task_name"],
	})
	if err != nil {
		return nil, fmt.Errorf("new error detail: %w", err)
	}

	return connectErr.WithDetail(detail), nil
}

// connectCodeOf maps a Kind to a Connect code.
func connectCodeOf(kind apierr.Kind) connect.Code {
	switch kind {
	case apierr.KindInvalid:
		return connect.CodeInvalidArgument
	case apierr.KindUnauthenticated:
		return connect.CodeUnauthenticated
	case apierr.KindNotFound:
		return connect.CodeNotFound
	case apierr.KindConflict:
		return connect.CodeAlreadyExists
	case apierr.KindFailedPrecondition:
		return connect.CodeFailedPrecondition
	case apierr.KindUnavailable:
		return connect.CodeUnavailable
	case apierr.KindCanceled:
		return connect.CodeCanceled
	case apierr.KindDeadlineExceeded:
		return connect.CodeDeadlineExceeded
	default:
		return connect.CodeInternal
	}
}

var errMissingBearer = apierr.Unauthenticated(apierr.CodeUnauthenticated, "missing bearer token")

// bearerToken returns the token of the request's Authorization header.
func bearerToken(ctx context.Context) (string, error) {
	info, ok := connect.CallInfoForServerContext(ctx)
	if !ok {
		return "", errMissingBearer
	}

	token, ok := strings.CutPrefix(info.RequestHeader().Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return "", errMissingBearer
	}

	return token, nil
}

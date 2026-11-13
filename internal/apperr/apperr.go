package apperr

import (
	"maps"
	"net/http"
)

// Kind is a semantic failure category. The zero value is KindInternal, so an Error
// built without a Kind is an internal failure.
type Kind int

const (
	// KindInternal is an unexpected failure. It is the zero value.
	KindInternal Kind = iota
	// KindInvalid is a malformed or rejected request.
	KindInvalid
	// KindUnauthenticated is a missing or wrong credential.
	KindUnauthenticated
	// KindPermissionDenied is a caller without the permission the request needs.
	KindPermissionDenied
	// KindNotFound is a missing resource.
	KindNotFound
	// KindConflict is a clash with existing state.
	KindConflict
	// KindFailedPrecondition is a request that the current state does not allow.
	KindFailedPrecondition
	// KindUnavailable is a transient failure that the caller may retry.
	KindUnavailable
	// KindCanceled is a call that the client gave up on.
	KindCanceled
	// KindDeadlineExceeded is a call whose client deadline passed before it finished.
	KindDeadlineExceeded
)

// HTTPStatus returns the HTTP status of k. A Kind the dashboard API never returns,
// such as KindCanceled, is 500.
func (k Kind) HTTPStatus() int {
	switch k {
	case KindInvalid:
		return http.StatusBadRequest
	case KindUnauthenticated:
		return http.StatusUnauthorized
	case KindPermissionDenied:
		return http.StatusForbidden
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict, KindFailedPrecondition:
		return http.StatusConflict
	case KindUnavailable:
		return http.StatusServiceUnavailable
	case KindDeadlineExceeded:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// Error is the structured error the core returns. Code identifies the failure
// for the client; it is empty for failures that have no schema code. Message is
// the text for people. Details carry identifiers such as "run_id" and "task_id".
type Error struct {
	Kind    Kind
	Code    Code
	Message string
	Details map[string]string
}

// Error returns the message. A nil Error has an empty message.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}

	return e.Message
}

// WithDetail returns a new error with a copied Details map and the added detail.
func (e *Error) WithDetail(key, value string) *Error {
	details := maps.Clone(e.Details)
	if details == nil {
		details = make(map[string]string)
	}

	details[key] = value
	return &Error{Kind: e.Kind, Code: e.Code, Message: e.Message, Details: details}
}

func newError(kind Kind, code Code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// Invalid is a malformed or rejected request.
func Invalid(code Code, message string) *Error {
	return newError(KindInvalid, code, message)
}

// Unauthenticated is a missing or wrong credential.
func Unauthenticated(code Code, message string) *Error {
	return newError(KindUnauthenticated, code, message)
}

// PermissionDenied is a caller without the permission the request needs.
func PermissionDenied(code Code, message string) *Error {
	return newError(KindPermissionDenied, code, message)
}

// NotFound is a missing resource.
func NotFound(code Code, message string) *Error {
	return newError(KindNotFound, code, message)
}

// Conflict is a clash with existing state.
func Conflict(code Code, message string) *Error {
	return newError(KindConflict, code, message)
}

// FailedPrecondition is a request that the current state does not allow.
func FailedPrecondition(code Code, message string) *Error {
	return newError(KindFailedPrecondition, code, message)
}

// Unavailable is a transient failure that the caller may retry.
func Unavailable(code Code, message string) *Error {
	return newError(KindUnavailable, code, message)
}

// Canceled is a call that the client gave up on. It has no schema code.
func Canceled(message string) *Error {
	return newError(KindCanceled, "", message)
}

// DeadlineExceeded is a call whose client deadline passed before it finished. It has no schema code.
func DeadlineExceeded(message string) *Error {
	return newError(KindDeadlineExceeded, "", message)
}

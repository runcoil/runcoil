package apierr

// Code is the stable identifier of an API error. The dashboard API sends it in lower case,
// as an ErrorCode of its schema.
type Code string

// The codes of the API. A Code the dashboard API returns is in its schema's ErrorCode.
const (
	CodeInvalidRequest       Code = "INVALID_REQUEST"
	CodeUnauthenticated      Code = "UNAUTHENTICATED"
	CodeIncompatibleProtocol Code = "INCOMPATIBLE_PROTOCOL"
	CodeInvalidRegistration  Code = "INVALID_REGISTRATION"
	CodeTaskNotFound         Code = "TASK_NOT_FOUND"
	CodeRunNotFound          Code = "RUN_NOT_FOUND"
	CodeIdempotencyConflict  Code = "IDEMPOTENCY_CONFLICT"
	CodeInputTooLarge        Code = "INPUT_TOO_LARGE"
	CodeNotRegistered        Code = "NOT_REGISTERED"
	CodeStaleLease           Code = "STALE_LEASE"
	CodeRunNotFinished       Code = "RUN_NOT_FINISHED"
	CodeStepNotFound         Code = "STEP_NOT_FOUND"
	CodePermissionDenied     Code = "PERMISSION_DENIED"
)

// Codes lists every Code, so that a test can check each one against the schema.
var Codes = []Code{
	CodeInvalidRequest,
	CodeUnauthenticated,
	CodeIncompatibleProtocol,
	CodeInvalidRegistration,
	CodeTaskNotFound,
	CodeRunNotFound,
	CodeIdempotencyConflict,
	CodeInputTooLarge,
	CodeNotRegistered,
	CodeStaleLease,
	CodeRunNotFinished,
	CodeStepNotFound,
	CodePermissionDenied,
}

package middleware

import (
	"context"

	"github.com/runcoil/runcoil/internal/domain"
)

// Caller is the signed-in user of a request and the session token it came with.
type Caller struct {
	User  domain.User
	Token string
}

type callerKey struct{}

func withCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// CallerFromContext returns the caller that LoadSession put in ctx. It reports false
// when the request has no live session.
func CallerFromContext(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerKey{}).(Caller)
	return c, ok
}

type requestInfo struct {
	err    error
	stack  string
	userID string
}

type requestInfoKey struct{}

func withRequestInfo(ctx context.Context, info *requestInfo) context.Context {
	return context.WithValue(ctx, requestInfoKey{}, info)
}

// RecordError puts err on the log line of the request ctx belongs to.
func RecordError(ctx context.Context, err error) {
	if info, ok := ctx.Value(requestInfoKey{}).(*requestInfo); ok {
		info.err = err
	}
}

func recordPanic(ctx context.Context, err error, stack string) {
	if info, ok := ctx.Value(requestInfoKey{}).(*requestInfo); ok {
		info.err = err
		info.stack = stack
	}
}

func recordUserID(ctx context.Context, userID string) {
	if info, ok := ctx.Value(requestInfoKey{}).(*requestInfo); ok {
		info.userID = userID
	}
}

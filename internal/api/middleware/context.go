package middleware

import (
	"context"

	"github.com/runcoil/runcoil/internal/domain"
)

type identityKey struct{}

func withIdentity(ctx context.Context, identity domain.Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

// IdentityFromContext returns the identity that Authenticate put in ctx. It reports false
// when the request has no valid credential.
func IdentityFromContext(ctx context.Context) (domain.Identity, bool) {
	identity, ok := ctx.Value(identityKey{}).(domain.Identity)
	return identity, ok
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

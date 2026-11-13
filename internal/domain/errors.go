package domain

import "errors"

// The outcomes a store reports. The services pick what the client sees.
var (
	ErrAPIKeyNotFound  = errors.New("api key not found")
	ErrUserNotFound    = errors.New("user not found")
	ErrMemberNotFound  = errors.New("project member not found")
	ErrSessionNotFound = errors.New("no session for the token")
	ErrSessionEnded    = errors.New("worker session has ended")
	ErrStaleLease      = errors.New("lease is no longer the attempt's")
	ErrStreamFenced    = errors.New("worker's stream is no longer this one")
)

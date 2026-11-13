package domain

import "errors"

// The outcomes the user store reports. The services pick what the client sees.
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrPasswordChanged = errors.New("password changed since it was checked")
)

// The outcomes the session store reports.
var (
	ErrSessionNotFound = errors.New("no session for the token")
)

// The outcomes the worker store reports.
var (
	ErrAPIKeyNotFound = errors.New("api key not found")
	ErrSessionEnded   = errors.New("worker session has ended")
	ErrStaleLease     = errors.New("lease is no longer the attempt's")
	ErrStreamFenced   = errors.New("worker's stream is no longer this one")
)

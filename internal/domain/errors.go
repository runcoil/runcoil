package domain

import "errors"

// The outcomes a store reports. The dispatcher picks what the client sees.
var (
	ErrAPIKeyNotFound = errors.New("api key not found")
	ErrSessionEnded   = errors.New("worker session has ended")
	ErrStaleLease     = errors.New("lease is no longer the attempt's")
	ErrStreamFenced   = errors.New("worker's stream is no longer this one")
)

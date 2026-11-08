package domain

import "errors"

// The outcomes a store reports. Only the engine knows what is missing or stale, so it
// picks the error the client sees.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrStale    = errors.New("stale")
)

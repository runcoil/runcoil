package domain

import "uuid"

// User is a dashboard account.
type User struct {
	ID           uuid.UUID
	Username     string
	PasswordHash string
}

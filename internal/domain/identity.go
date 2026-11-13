package domain

import "uuid"

// CredentialType says how a client proves who it is.
type CredentialType string

// The types of a credential.
const (
	CredentialSession CredentialType = "session"
)

// Credential is what a client sends to prove who it is.
type Credential struct {
	Type   CredentialType
	Secret string
}

// IdentityType says what kind of caller an Identity is.
type IdentityType string

// The types of an identity.
const (
	IdentityUser IdentityType = "user"
)

// Identity is who a request comes from, once its credential is checked, and what it may
// do anywhere on the server. A permission inside a project is checked against the
// project's role.
type Identity struct {
	Type   IdentityType
	UserID uuid.UUID
	// SessionID is the key of the session, the sha256 of its token.
	SessionID   []byte
	Permissions []Permission
}

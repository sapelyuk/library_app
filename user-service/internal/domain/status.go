package domain

import "strings"

// UserStatus is the lifecycle state of an account.
type UserStatus string

const (
	// UserStatusActive allows signing in.
	UserStatusActive UserStatus = "ACTIVE"

	// UserStatusDeactivated blocks the account, the data is kept.
	UserStatusDeactivated UserStatus = "DEACTIVATED"
)

// Valid reports whether the status is known.
func (s UserStatus) Valid() bool {
	switch s {
	case UserStatusActive, UserStatusDeactivated:
		return true
	default:
		return false
	}
}

// ParseStatus converts a stored or submitted string into a status.
func ParseStatus(raw string) (UserStatus, error) {
	status := UserStatus(normalizeToken(raw))
	if !status.Valid() {
		return "", ErrInvalidStatus
	}

	return status, nil
}

// normalizeToken upper cases a stored enum value so that the database may keep
// any historical spelling.
func normalizeToken(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}

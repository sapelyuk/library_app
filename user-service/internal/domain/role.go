package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Role is the access level of a user inside the library.
//
// The set is intentionally small: a reader owns an account, a librarian owns
// the accounts of the readers. Anything finer belongs to the service that owns
// the data being protected, not to the identity.
type Role string

const (
	// RoleReader is a library visitor: borrows books, manages own account.
	RoleReader Role = "READER"

	// RoleLibrarian is a staff member: manages accounts and the catalog.
	RoleLibrarian Role = "LIBRARIAN"
)

// Valid reports whether the role is known.
func (r Role) Valid() bool {
	switch r {
	case RoleReader, RoleLibrarian:
		return true
	default:
		return false
	}
}

// AtLeast reports whether the role has the authority of min or more. Unknown
// roles rank below everything, so a broken value never escalates.
func (r Role) AtLeast(min Role) bool {
	return roleRank(r) >= roleRank(min)
}

// ParseRole normalises and validates a role coming from a transport or from the
// storage.
func ParseRole(raw string) (Role, error) {
	role := Role(strings.ToUpper(strings.TrimSpace(raw)))
	if !role.Valid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidRole, raw)
	}

	return role, nil
}

func roleRank(role Role) int {
	switch role {
	case RoleReader:
		return 1
	case RoleLibrarian:
		return 2
	default:
		return 0
	}
}

// countRunes is a tiny helper that keeps the length checks below honest: the
// limits of a profile are counted in characters, not in bytes.
func countRunes(value string) int {
	return utf8.RuneCountInString(value)
}

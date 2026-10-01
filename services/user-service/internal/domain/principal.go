package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// Principal is the authenticated caller: the identity resolved from a bearer
// token together with the session it came from.
//
// Every privileged use case takes it as an explicit argument instead of digging
// it out of the context, so the authorization rules are visible in the
// signature of the method and are the same for gRPC and for REST.
type Principal struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Email     Email
	Role      Role
}

// IsLibrarian reports whether the caller holds the librarian role.
func (p Principal) IsLibrarian() bool {
	return p.Role.AtLeast(RoleLibrarian)
}

// IsSelf reports whether the target account belongs to the caller.
func (p Principal) IsSelf(target uuid.UUID) bool {
	return p.UserID == target
}

// RequireLibrarian guards the operations that belong to the staff only. The
// action names the operation in the message, which keeps a denial readable in
// the log of the caller.
func (p Principal) RequireLibrarian(action string) error {
	if !p.IsLibrarian() {
		return fmt.Errorf("%w: %s requires the librarian role", ErrPermissionDenied, action)
	}

	return nil
}

// RequireAccountAccess guards GetUser: a reader sees own account, a librarian
// sees any account.
func (p Principal) RequireAccountAccess(target uuid.UUID) error {
	if p.IsSelf(target) || p.IsLibrarian() {
		return nil
	}

	return fmt.Errorf("%w: a reader may read only own account", ErrPermissionDenied)
}

// RequireUpdateAccess guards UpdateUser. Contact details belong to the owner,
// the login identifier, the role and the status belong to the staff. A
// librarian may not change role or status on own account either: that keeps
// both privilege escalation and self-lockout out of the system.
func (p Principal) RequireUpdateAccess(target uuid.UUID, update UserUpdate) error {
	if p.IsSelf(target) {
		if update.Email != nil || update.Role != nil || update.Status != nil {
			return fmt.Errorf(
				"%w: the email, the role and the status are changed by another librarian",
				ErrPermissionDenied,
			)
		}

		return nil
	}

	return p.RequireLibrarian("updating another account")
}

// RequirePasswordChange guards ChangePassword. The owner proves the current
// password; resetting the password of somebody else is the job of a librarian.
func (p Principal) RequirePasswordChange(target uuid.UUID, byOwner bool) error {
	if byOwner {
		if !p.IsSelf(target) {
			return fmt.Errorf("%w: the current password is proven only by the owner", ErrPermissionDenied)
		}

		return nil
	}

	return p.RequireLibrarian("resetting the password of another account")
}

// RequireStatusChange guards DeactivateUser and RestoreUser. A librarian who
// can block own account is one typo away from a library without staff.
func (p Principal) RequireStatusChange(target uuid.UUID) error {
	if err := p.RequireLibrarian("changing the status of an account"); err != nil {
		return err
	}

	if p.IsSelf(target) {
		return fmt.Errorf("%w: a librarian cannot change own status", ErrSelfLockout)
	}

	return nil
}

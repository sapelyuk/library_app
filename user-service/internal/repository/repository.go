// Package repository defines the storage contracts of the user service.
//
// The contracts speak domain types only: no SQL, no driver errors, no row
// structs. An implementation is required to translate a storage failure into a
// domain error, so that the service layer can act on "this email is taken"
// without knowing that a UNIQUE index is what said so.
package repository

import (
	"context"

	"github.com/google/uuid"

	"library_app/user-service/internal/domain"
)

// UserRepository stores the accounts of readers and librarians.
type UserRepository interface {
	// Create inserts an account. A duplicate email must come back as
	// domain.ErrEmailAlreadyExists.
	Create(ctx context.Context, user *domain.User) error

	// Update writes the mutable columns of an account. An account that
	// disappeared in between must come back as domain.ErrUserNotFound.
	Update(ctx context.Context, user *domain.User) error

	// GetByID returns domain.ErrUserNotFound when there is no such account.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)

	// GetByEmail returns domain.ErrUserNotFound when there is no such account.
	// The email arrives normalized from the domain, the lookup hits the unique
	// index over the stored value.
	GetByEmail(ctx context.Context, email domain.Email) (*domain.User, error)

	// List returns one page of accounts and the total number of rows the filter
	// matches, which is what the pager of the UI needs.
	List(ctx context.Context, filter domain.UserFilter) ([]*domain.User, int, error)
}

// SessionRepository stores the bearer tokens of the service.
//
// A token reaches this layer only as the SHA-256 digest produced by
// internal/security, so a dump of the table cannot be replayed against the API.
type SessionRepository interface {
	// Create stores a freshly issued session.
	Create(ctx context.Context, session *domain.Session) error

	// GetByTokenHash returns domain.ErrSessionNotFound for a token that was
	// never issued or has already been revoked. Whether a found session is still
	// alive is decided by the service, not here.
	GetByTokenHash(ctx context.Context, tokenHash string) (*domain.Session, error)

	// DeleteByTokenHash logs one device out. It is idempotent on purpose:
	// signing out twice is not an error for the caller.
	DeleteByTokenHash(ctx context.Context, tokenHash string) error

	// DeleteByUser logs every device of one account out, which is what blocking
	// an account implies. It is idempotent.
	DeleteByUser(ctx context.Context, userID uuid.UUID) error

	// DeleteByUserExcept logs every device of one account out but the session
	// that is still in use: a password change must not sign out the caller who
	// just proved the old password.
	DeleteByUserExcept(ctx context.Context, userID, except uuid.UUID) error
}

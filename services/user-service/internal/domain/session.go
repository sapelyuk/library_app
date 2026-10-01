package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Session is a bearer token of a user. Only the SHA-256 hash of the token is
// stored: a leaked table must not be usable as a set of live credentials.
type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// NewSession wraps a freshly generated token hash. The caller generates the
// token in internal/security and passes the hash here, because the domain has
// no business knowing how a token is produced.
func NewSession(userID uuid.UUID, tokenHash string, ttl time.Duration, now time.Time) *Session {
	return &Session{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: strings.TrimSpace(tokenHash),
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
}

// ExpiredAt reports whether the session lived past the given moment.
func (s *Session) ExpiredAt(now time.Time) bool {
	return !now.Before(s.ExpiresAt)
}

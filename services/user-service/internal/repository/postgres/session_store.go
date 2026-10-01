package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/sapelyuk/smart-library/services/user-service/internal/domain"
	"github.com/sapelyuk/smart-library/services/user-service/internal/repository"
)

// SessionStore is the PostgreSQL implementation of
// repository.SessionRepository.
//
// Sessions expire by their own column rather than by a TTL in Redis: signing a
// token out has to survive a restart of the cache, and the read of one indexed
// row per request is not what this service needs to optimise. A purge job sweeps
// the dead rows.
type SessionStore struct {
	db *sql.DB
}

var _ repository.SessionRepository = (*SessionStore)(nil)

// NewSessionStore wraps an opened pool.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

const sessionColumns = `id, user_id, token_hash, created_at, expires_at`

// Create stores an issued session.
func (s *SessionStore) Create(ctx context.Context, session *domain.Session) error {
	const query = `
		INSERT INTO sessions (id, user_id, token_hash, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := s.db.ExecContext(ctx, query,
		session.ID, session.UserID, session.TokenHash, session.CreatedAt, session.ExpiresAt,
	)

	return translate(err, "")
}

// GetByTokenHash returns the session a bearer token stands for.
func (s *SessionStore) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.Session, error) {
	query := `SELECT ` + sessionColumns + ` FROM sessions WHERE token_hash = $1`

	var session domain.Session

	err := s.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&session.ID, &session.UserID, &session.TokenHash, &session.CreatedAt, &session.ExpiresAt,
	)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, domain.ErrSessionNotFound
	case err != nil:
		return nil, fmt.Errorf("postgres: query session: %w", err)
	default:
		return &session, nil
	}
}

// DeleteByTokenHash logs one device out. A token that is already gone is not an
// error, which is what makes the endpoint safe to call twice.
func (s *SessionStore) DeleteByTokenHash(ctx context.Context, tokenHash string) error {
	const query = `DELETE FROM sessions WHERE token_hash = $1`

	if _, err := s.db.ExecContext(ctx, query, tokenHash); err != nil {
		return translate(err, "")
	}

	return nil
}

// DeleteByUser logs every device of one account out.
func (s *SessionStore) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	const query = `DELETE FROM sessions WHERE user_id = $1`

	if _, err := s.db.ExecContext(ctx, query, userID); err != nil {
		return translate(err, "")
	}

	return nil
}

// DeleteByUserExcept logs every device of one account out but the given session.
func (s *SessionStore) DeleteByUserExcept(ctx context.Context, userID, except uuid.UUID) error {
	const query = `DELETE FROM sessions WHERE user_id = $1 AND id <> $2`

	if _, err := s.db.ExecContext(ctx, query, userID, except); err != nil {
		return translate(err, "")
	}

	return nil
}

// PurgeExpired removes the sessions whose expiry has passed and reports how many
// rows went away. The maintenance loop of the service calls it on a ticker, which
// is why the statement is written to be safe to run at any moment.
func (s *SessionStore) PurgeExpired(ctx context.Context, now time.Time) (int64, error) {
	const query = `DELETE FROM sessions WHERE expires_at <= $1`

	result, err := s.db.ExecContext(ctx, query, now)
	if err != nil {
		return 0, translate(err, "")
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("postgres: purge sessions: %w", err)
	}

	return deleted, nil
}

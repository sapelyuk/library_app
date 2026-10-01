package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sapelyuk/smart-library/services/user-service/internal/domain"
)

func TestNewSession(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	now := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	ttl := time.Hour

	session := domain.NewSession(userID, "  abc123  ", ttl, now)

	if session.ID.String() == "" {
		t.Fatal("ID must be generated")
	}

	if session.UserID != userID {
		t.Fatalf("UserID = %s, want %s", session.UserID, userID)
	}

	if session.TokenHash != "abc123" {
		t.Fatalf("TokenHash = %q, want %q", session.TokenHash, "abc123")
	}

	if !session.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %v, want %v", session.CreatedAt, now)
	}

	if !session.ExpiresAt.Equal(now.Add(ttl)) {
		t.Fatalf("ExpiresAt = %v, want %v", session.ExpiresAt, now.Add(ttl))
	}
}

func TestSessionExpiredAt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	session := domain.NewSession(uuid.New(), "hash", time.Hour, now)

	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{name: "at creation", at: now, want: false},
		{name: "before the deadline", at: now.Add(59 * time.Minute), want: false},
		{name: "exactly at the deadline", at: now.Add(time.Hour), want: true},
		{name: "after the deadline", at: now.Add(2 * time.Hour), want: true},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := session.ExpiredAt(tc.at); got != tc.want {
				t.Fatalf("ExpiredAt(%v) = %v, want %v", tc.at, got, tc.want)
			}
		})
	}
}

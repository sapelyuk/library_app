package domain_test

import (
	"errors"
	"testing"

	"library_app/user-service/internal/domain"
)

func TestUserStatusValid(t *testing.T) {
	t.Parallel()

	if !domain.UserStatusActive.Valid() {
		t.Fatal("ACTIVE should be valid")
	}
	if !domain.UserStatusDeactivated.Valid() {
		t.Fatal("DEACTIVATED should be valid")
	}
	if domain.UserStatus("").Valid() {
		t.Fatal("empty status should be invalid")
	}
	if domain.UserStatus("BANNED").Valid() {
		t.Fatal("unknown status should be invalid")
	}
	if domain.UserStatus("active").Valid() {
		t.Fatal("lowercase status should be invalid")
	}
}

func TestParseStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    domain.UserStatus
		wantErr error
	}{
		{name: "active", raw: "ACTIVE", want: domain.UserStatusActive},
		{name: "normalized from storage", raw: "  active  ", want: domain.UserStatusActive},
		{name: "deactivated", raw: "deactivated", want: domain.UserStatusDeactivated},
		{name: "empty", raw: "", wantErr: domain.ErrInvalidStatus},
		{name: "unknown", raw: "banned", wantErr: domain.ErrInvalidStatus},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.ParseStatus(tc.raw)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

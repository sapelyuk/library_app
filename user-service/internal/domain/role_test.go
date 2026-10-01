package domain_test

import (
	"errors"
	"testing"

	"library_app/user-service/internal/domain"
)

func TestRoleValid(t *testing.T) {
	t.Parallel()

	if !domain.RoleReader.Valid() {
		t.Fatal("RoleReader should be valid")
	}
	if !domain.RoleLibrarian.Valid() {
		t.Fatal("RoleLibrarian should be valid")
	}
	if domain.Role("").Valid() {
		t.Fatal("empty role should be invalid")
	}
	if domain.Role("ADMIN").Valid() {
		t.Fatal("unknown role should be invalid")
	}
	if domain.Role("reader").Valid() {
		t.Fatal("lowercase role should be invalid")
	}
}

func TestRoleAtLeast(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role domain.Role
		min  domain.Role
		want bool
	}{
		{domain.RoleReader, domain.RoleReader, true},
		{domain.RoleLibrarian, domain.RoleReader, true},
		{domain.RoleReader, domain.RoleLibrarian, false},
		{domain.RoleLibrarian, domain.RoleLibrarian, true},
		{domain.Role("GOD"), domain.RoleReader, false},
		{domain.Role("GOD"), domain.RoleLibrarian, false},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(string(tc.role)+"->"+string(tc.min), func(t *testing.T) {
			t.Parallel()

			if got := tc.role.AtLeast(tc.min); got != tc.want {
				t.Fatalf("AtLeast(%s) = %v, want %v", tc.min, got, tc.want)
			}
		})
	}
}

func TestParseRole(t *testing.T) {
	t.Parallel()

	got, err := domain.ParseRole("  reader  ")
	if err != nil {
		t.Fatalf("ParseRole: unexpected error: %v", err)
	}
	if got != domain.RoleReader {
		t.Fatalf("got %q, want %q", got, domain.RoleReader)
	}

	got, err = domain.ParseRole("LIBRARIAN")
	if err != nil {
		t.Fatalf("ParseRole: unexpected error: %v", err)
	}
	if got != domain.RoleLibrarian {
		t.Fatalf("got %q, want %q", got, domain.RoleLibrarian)
	}

	_, err = domain.ParseRole("")
	if !errors.Is(err, domain.ErrInvalidRole) {
		t.Fatalf("want ErrInvalidRole, got %v", err)
	}

	_, err = domain.ParseRole("admin")
	if !errors.Is(err, domain.ErrInvalidRole) {
		t.Fatalf("want ErrInvalidRole, got %v", err)
	}
}

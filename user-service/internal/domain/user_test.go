package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"library_app/user-service/internal/domain"
)

// fakeHash stands in for the argon2id encoding produced by internal/security:
// the domain only refuses an empty or absurd hash, it never parses one.
const fakeHash = "$argon2id$v=19$m=19456,t=2,p=1$ZmFrZXNhbHQ$ZmFrZWtleQ"

func newValidUser(t *testing.T) *domain.User {
	t.Helper()

	user, err := domain.NewUser("  Reader@Example.COM ", fakeHash, "  John   Doe ", "+1 (555) 123-45-67", domain.RoleReader)
	if err != nil {
		t.Fatalf("NewUser: unexpected error: %v", err)
	}

	return user
}

func TestNewUser(t *testing.T) {
	t.Parallel()

	user := newValidUser(t)

	if user.ID.String() == "" {
		t.Fatal("ID must be generated")
	}

	if user.Email != "reader@example.com" {
		t.Fatalf("Email = %q, want %q", user.Email, "reader@example.com")
	}

	if user.FullName != "John Doe" {
		t.Fatalf("FullName = %q, want %q", user.FullName, "John Doe")
	}

	if user.Phone != "+15551234567" {
		t.Fatalf("Phone = %q, want %q", user.Phone, "+15551234567")
	}

	if user.Role != domain.RoleReader {
		t.Fatalf("Role = %q, want %q", user.Role, domain.RoleReader)
	}

	if user.Status != domain.UserStatusActive {
		t.Fatalf("Status = %q, want %q", user.Status, domain.UserStatusActive)
	}

	if !user.IsActive() {
		t.Fatal("a fresh account must be active")
	}

	if !user.LastLogin.IsZero() {
		t.Fatalf("LastLogin = %v, want the zero time", user.LastLogin)
	}
}

func TestNewUserGeneratesDistinctIDs(t *testing.T) {
	t.Parallel()

	first := newValidUser(t)
	second := newValidUser(t)

	if first.ID == second.ID {
		t.Fatal("two accounts must not share an ID")
	}
}

func TestNewUserErrors(t *testing.T) {
	t.Parallel()

	tooLongHash := strings.Repeat("h", 513)

	tests := []struct {
		name    string
		email   string
		hash    string
		full    string
		phone   string
		role    domain.Role
		wantErr error
	}{
		{
			name:    "invalid email",
			email:   "not-an-email",
			hash:    fakeHash,
			full:    "John Doe",
			role:    domain.RoleReader,
			wantErr: domain.ErrInvalidEmail,
		},
		{
			name:    "empty password hash",
			email:   "reader@example.com",
			hash:    "",
			full:    "John Doe",
			role:    domain.RoleReader,
			wantErr: domain.ErrPasswordTooWeak,
		},
		{
			name:    "absurdly long password hash",
			email:   "reader@example.com",
			hash:    tooLongHash,
			full:    "John Doe",
			role:    domain.RoleReader,
			wantErr: domain.ErrPasswordTooWeak,
		},
		{
			name:    "empty full name",
			email:   "reader@example.com",
			hash:    fakeHash,
			full:    "   ",
			role:    domain.RoleReader,
			wantErr: domain.ErrEmptyFullName,
		},
		{
			name:    "unsupported character in full name",
			email:   "reader@example.com",
			hash:    fakeHash,
			full:    "John@Doe",
			role:    domain.RoleReader,
			wantErr: domain.ErrInvalidFullName,
		},
		{
			name:    "phone with too few digits",
			email:   "reader@example.com",
			hash:    fakeHash,
			full:    "John Doe",
			phone:   "12345",
			role:    domain.RoleReader,
			wantErr: domain.ErrInvalidPhone,
		},
		{
			name:    "unknown role",
			email:   "reader@example.com",
			hash:    fakeHash,
			full:    "John Doe",
			role:    domain.Role("ADMIN"),
			wantErr: domain.ErrInvalidRole,
		},
		{
			name:    "empty role",
			email:   "reader@example.com",
			hash:    fakeHash,
			full:    "John Doe",
			role:    domain.Role(""),
			wantErr: domain.ErrInvalidRole,
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			user, err := domain.NewUser(tc.email, tc.hash, tc.full, tc.phone, tc.role)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}

			if user != nil {
				t.Fatalf("rejected input must not produce a user, got %+v", user)
			}
		})
	}
}

func TestSetPasswordHash(t *testing.T) {
	t.Parallel()

	user := newValidUser(t)

	const replacement = "$argon2id$v=19$m=19456,t=2,p=1$bmV3c2FsdA$bmV3a2V5"

	if err := user.SetPasswordHash(replacement); err != nil {
		t.Fatalf("SetPasswordHash: unexpected error: %v", err)
	}

	if user.PasswordHash != replacement {
		t.Fatalf("PasswordHash = %q, want %q", user.PasswordHash, replacement)
	}

	if err := user.SetPasswordHash("   "); !errors.Is(err, domain.ErrPasswordTooWeak) {
		t.Fatalf("want ErrPasswordTooWeak, got %v", err)
	}

	if user.PasswordHash != replacement {
		t.Fatalf("a rejected hash must not overwrite the current one, got %q", user.PasswordHash)
	}
}

func TestUserLifecycle(t *testing.T) {
	t.Parallel()

	user := newValidUser(t)

	if user.IsActive() != true {
		t.Fatal("a fresh account must be active")
	}

	user.Deactivate()
	if user.IsActive() {
		t.Fatal("a deactivated account must not be active")
	}
	if user.Status != domain.UserStatusDeactivated {
		t.Fatalf("Status = %q, want %q", user.Status, domain.UserStatusDeactivated)
	}

	user.Restore()
	if !user.IsActive() {
		t.Fatal("a restored account must be active again")
	}
	if user.Status != domain.UserStatusActive {
		t.Fatalf("Status = %q, want %q", user.Status, domain.UserStatusActive)
	}
}

func TestUserMarkLogin(t *testing.T) {
	t.Parallel()

	user := newValidUser(t)

	if !user.LastLogin.IsZero() {
		t.Fatal("LastLogin must start zero")
	}

	at := time.Now().UTC()
	user.MarkLogin(at)

	if !user.LastLogin.Equal(at) {
		t.Fatalf("LastLogin = %v, want %v", user.LastLogin, at)
	}
}

func TestUserApply(t *testing.T) {
	t.Parallel()

	role := domain.RoleLibrarian
	status := domain.UserStatusDeactivated
	mail := "updated@example.com"
	name := "  Jane   Roe "
	phone := "+7 (999) 123-45-67"

	tests := []struct {
		name    string
		update  domain.UserUpdate
		wantErr error
		check   func(t *testing.T, user *domain.User)
	}{
		{
			name:   "empty update changes nothing",
			update: domain.UserUpdate{},
			check: func(t *testing.T, user *domain.User) {
				if user.FullName != "John Doe" || user.Role != domain.RoleReader {
					t.Fatalf("empty update must not touch the account, got %+v", user)
				}
			},
		},
		{
			name:   "contact details are updated",
			update: domain.UserUpdate{FullName: &name, Phone: &phone},
			check: func(t *testing.T, user *domain.User) {
				if user.FullName != "Jane Roe" {
					t.Fatalf("FullName = %q, want %q", user.FullName, "Jane Roe")
				}
				if user.Phone != "+79991234567" {
					t.Fatalf("Phone = %q, want %q", user.Phone, "+79991234567")
				}
			},
		},
		{
			name:   "role and status are updated",
			update: domain.UserUpdate{Role: &role, Status: &status},
			check: func(t *testing.T, user *domain.User) {
				if user.Role != domain.RoleLibrarian {
					t.Fatalf("Role = %q, want %q", user.Role, domain.RoleLibrarian)
				}
				if user.Status != domain.UserStatusDeactivated {
					t.Fatalf("Status = %q, want %q", user.Status, domain.UserStatusDeactivated)
				}
			},
		},
		{
			name:   "email is normalized",
			update: domain.UserUpdate{Email: &mail},
			check: func(t *testing.T, user *domain.User) {
				if user.Email != "updated@example.com" {
					t.Fatalf("Email = %q, want %q", user.Email, "updated@example.com")
				}
			},
		},
		{
			name:    "invalid email rejects the whole update",
			update:  domain.UserUpdate{FullName: &name, Email: strPtr("broken")},
			wantErr: domain.ErrInvalidEmail,
			check: func(t *testing.T, user *domain.User) {
				if user.FullName != "John Doe" {
					t.Fatalf("a rejected update must leave the account untouched, got %q", user.FullName)
				}
			},
		},
		{
			name:    "invalid role rejects the whole update",
			update:  domain.UserUpdate{Role: rolePtr(domain.Role("ADMIN"))},
			wantErr: domain.ErrInvalidRole,
		},
		{
			name:    "invalid status rejects the whole update",
			update:  domain.UserUpdate{Status: statusPtr(domain.UserStatus("BANNED"))},
			wantErr: domain.ErrInvalidStatus,
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			user := newValidUser(t)

			err := user.Apply(tc.update)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.check != nil {
				tc.check(t, user)
			}
		})
	}
}

func TestUserUpdateIsEmpty(t *testing.T) {
	t.Parallel()

	if !(domain.UserUpdate{}).IsEmpty() {
		t.Fatal("the zero update must be empty")
	}

	mail := "reader@example.com"

	if (domain.UserUpdate{Email: &mail}).IsEmpty() {
		t.Fatal("an update with a field must not be empty")
	}
}

func TestValidateFullName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr error
	}{
		{name: "whitespace is collapsed", raw: "  John   Doe  ", want: "John Doe"},
		{name: "hyphen and apostrophe", raw: "Jean-Luc O'Brien", want: "Jean-Luc O'Brien"},
		{name: "initials with dots", raw: "Anna M.", want: "Anna M."},
		{name: "digits are allowed", raw: "Reader 42", want: "Reader 42"},
		{name: "minimal two characters", raw: "Jo", want: "Jo"},
		{name: "maximal length", raw: strings.Repeat("a", 120), want: strings.Repeat("a", 120)},
		{name: "empty", raw: "   ", wantErr: domain.ErrEmptyFullName},
		{name: "single character", raw: "J", wantErr: domain.ErrEmptyFullName},
		{name: "too long", raw: strings.Repeat("a", 121), wantErr: domain.ErrInvalidFullName},
		{name: "unsupported character", raw: "John@Doe", wantErr: domain.ErrInvalidFullName},
		{name: "cyrillic is rejected", raw: "Иван Петров", wantErr: domain.ErrInvalidFullName},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.ValidateFullName(tc.raw)

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

func TestValidatePhone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr error
	}{
		{name: "formatting characters are dropped", raw: "+1 (555) 123-45-67", want: "+15551234567"},
		{name: "russian number", raw: "+7 (999) 123-45-67", want: "+79991234567"},
		{name: "digits only", raw: "5551234567", want: "5551234567"},
		{name: "minimal ten digits", raw: "1234567890", want: "1234567890"},
		{name: "maximal fifteen digits", raw: "123456789012345", want: "123456789012345"},
		{name: "spaces around are trimmed", raw: "  5551234567  ", want: "5551234567"},
		{name: "empty stays empty", raw: "", want: ""},
		{name: "blank stays empty", raw: "   ", want: ""},
		{name: "too few digits", raw: "123456789", wantErr: domain.ErrInvalidPhone},
		{name: "too many digits", raw: "1234567890123456", wantErr: domain.ErrInvalidPhone},
		{name: "letters", raw: "555-CALL-NOW", wantErr: domain.ErrInvalidPhone},
		{name: "plus in the middle", raw: "555+1234567", wantErr: domain.ErrInvalidPhone},
		{name: "input longer than the limit", raw: "+1 (555) 123-45-67 890123", wantErr: domain.ErrInvalidPhone},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.ValidatePhone(tc.raw)

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

func strPtr(value string) *string {
	return &value
}

func rolePtr(value domain.Role) *domain.Role {
	return &value
}

func statusPtr(value domain.UserStatus) *domain.UserStatus {
	return &value
}

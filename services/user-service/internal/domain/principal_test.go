package domain_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/sapelyuk/smart-library/services/user-service/internal/domain"
)

func readerPrincipal() domain.Principal {
	return domain.Principal{
		UserID:    uuid.New(),
		SessionID: uuid.New(),
		Email:     "reader@example.com",
		Role:      domain.RoleReader,
	}
}

func librarianPrincipal() domain.Principal {
	return domain.Principal{
		UserID:    uuid.New(),
		SessionID: uuid.New(),
		Email:     "librarian@library.local",
		Role:      domain.RoleLibrarian,
	}
}

func TestPrincipalIsLibrarian(t *testing.T) {
	t.Parallel()

	if readerPrincipal().IsLibrarian() {
		t.Fatal("a reader must not be a librarian")
	}

	if !librarianPrincipal().IsLibrarian() {
		t.Fatal("a librarian must be a librarian")
	}
}

func TestPrincipalIsSelf(t *testing.T) {
	t.Parallel()

	principal := readerPrincipal()

	if !principal.IsSelf(principal.UserID) {
		t.Fatal("the caller must be recognized as self")
	}

	if principal.IsSelf(uuid.New()) {
		t.Fatal("another account must not be recognized as self")
	}
}

func TestPrincipalRequireLibrarian(t *testing.T) {
	t.Parallel()

	if err := librarianPrincipal().RequireLibrarian("test action"); err != nil {
		t.Fatalf("a librarian must pass, got %v", err)
	}

	if err := readerPrincipal().RequireLibrarian("test action"); !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("want ErrPermissionDenied, got %v", err)
	}
}

func TestPrincipalRequireAccountAccess(t *testing.T) {
	t.Parallel()

	reader := readerPrincipal()
	librarian := librarianPrincipal()

	if err := reader.RequireAccountAccess(reader.UserID); err != nil {
		t.Fatalf("a reader must read own account, got %v", err)
	}

	if err := librarian.RequireAccountAccess(uuid.New()); err != nil {
		t.Fatalf("a librarian must read any account, got %v", err)
	}

	if err := reader.RequireAccountAccess(uuid.New()); !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("want ErrPermissionDenied, got %v", err)
	}
}

func TestPrincipalRequireUpdateAccess(t *testing.T) {
	t.Parallel()

	reader := readerPrincipal()
	librarian := librarianPrincipal()
	other := uuid.New()

	name := "New Name"
	mail := "new@example.com"
	role := domain.RoleLibrarian
	status := domain.UserStatusDeactivated

	tests := []struct {
		name    string
		by      domain.Principal
		target  uuid.UUID
		update  domain.UserUpdate
		wantErr error
	}{
		{
			name:   "reader updates own contact details",
			by:     reader,
			target: reader.UserID,
			update: domain.UserUpdate{FullName: &name},
		},
		{
			name:    "reader updates own email",
			by:      reader,
			target:  reader.UserID,
			update:  domain.UserUpdate{Email: &mail},
			wantErr: domain.ErrPermissionDenied,
		},
		{
			name:    "reader updates own role",
			by:      reader,
			target:  reader.UserID,
			update:  domain.UserUpdate{Role: &role},
			wantErr: domain.ErrPermissionDenied,
		},
		{
			name:    "reader updates own status",
			by:      reader,
			target:  reader.UserID,
			update:  domain.UserUpdate{Status: &status},
			wantErr: domain.ErrPermissionDenied,
		},
		{
			name:    "librarian does not change own role",
			by:      librarian,
			target:  librarian.UserID,
			update:  domain.UserUpdate{Role: &role},
			wantErr: domain.ErrPermissionDenied,
		},
		{
			name:    "librarian does not change own status",
			by:      librarian,
			target:  librarian.UserID,
			update:  domain.UserUpdate{Status: &status},
			wantErr: domain.ErrPermissionDenied,
		},
		{
			name:   "librarian updates another account",
			by:     librarian,
			target: other,
			update: domain.UserUpdate{Role: &role, Status: &status},
		},
		{
			name:    "reader updates another account",
			by:      reader,
			target:  other,
			update:  domain.UserUpdate{FullName: &name},
			wantErr: domain.ErrPermissionDenied,
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.by.RequireUpdateAccess(tc.target, tc.update)

			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				return
			}

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestPrincipalRequirePasswordChange(t *testing.T) {
	t.Parallel()

	reader := readerPrincipal()
	librarian := librarianPrincipal()

	if err := reader.RequirePasswordChange(reader.UserID, true); err != nil {
		t.Fatalf("the owner must change own password, got %v", err)
	}

	if err := librarian.RequirePasswordChange(librarian.UserID, true); err != nil {
		t.Fatalf("the librarian must change own password, got %v", err)
	}

	if err := reader.RequirePasswordChange(uuid.New(), true); !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("want ErrPermissionDenied, got %v", err)
	}

	if err := librarian.RequirePasswordChange(uuid.New(), false); err != nil {
		t.Fatalf("a librarian must reset another password, got %v", err)
	}

	if err := reader.RequirePasswordChange(uuid.New(), false); !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("want ErrPermissionDenied, got %v", err)
	}
}

func TestPrincipalRequireStatusChange(t *testing.T) {
	t.Parallel()

	reader := readerPrincipal()
	librarian := librarianPrincipal()

	if err := librarian.RequireStatusChange(uuid.New()); err != nil {
		t.Fatalf("a librarian must change another status, got %v", err)
	}

	if err := librarian.RequireStatusChange(librarian.UserID); !errors.Is(err, domain.ErrSelfLockout) {
		t.Fatalf("want ErrSelfLockout, got %v", err)
	}

	if err := reader.RequireStatusChange(uuid.New()); !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("want ErrPermissionDenied, got %v", err)
	}
}

package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/sapelyuk/smart-library/services/user-service/internal/domain"
)

func TestDefaultPasswordPolicy(t *testing.T) {
	t.Parallel()

	if got := domain.DefaultPasswordPolicy().MinLength; got != 12 {
		t.Fatalf("MinLength = %d, want 12", got)
	}
}

func TestPasswordPolicyValidate(t *testing.T) {
	t.Parallel()

	long := "Abcdef1!" + strings.Repeat("x", domain.MaxPasswordLength-8)

	tests := []struct {
		name    string
		policy  domain.PasswordPolicy
		raw     string
		wantErr error
	}{
		{
			name:   "default policy accepts a mixed password",
			policy: domain.DefaultPasswordPolicy(),
			raw:    "StrongPass1!",
		},
		{
			name:   "default policy accepts the maximal length",
			policy: domain.DefaultPasswordPolicy(),
			raw:    long,
		},
		{
			name:    "default policy rejects a shorter password",
			policy:  domain.DefaultPasswordPolicy(),
			raw:     "Short1!Ab",
			wantErr: domain.ErrPasswordTooShort,
		},
		{
			name:    "default policy rejects a password longer than the maximum",
			policy:  domain.DefaultPasswordPolicy(),
			raw:     long + "x",
			wantErr: domain.ErrPasswordTooLong,
		},
		{
			name:   "explicit minimum of eight is accepted",
			policy: domain.PasswordPolicy{MinLength: 8},
			raw:    "Abcdefg1",
		},
		{
			name:    "explicit minimum below the absolute floor is clamped",
			policy:  domain.PasswordPolicy{MinLength: 4},
			raw:     "Ab1cdef",
			wantErr: domain.ErrPasswordTooShort,
		},
		{
			name:   "zero minimum falls back to the absolute floor",
			policy: domain.PasswordPolicy{},
			raw:    "Abcdefg1",
		},
		{
			name:    "spaces are rejected",
			policy:  domain.DefaultPasswordPolicy(),
			raw:     "Strong Pass1",
			wantErr: domain.ErrPasswordTooWeak,
		},
		{
			name:    "tabs are rejected",
			policy:  domain.DefaultPasswordPolicy(),
			raw:     "Strong\tPass1",
			wantErr: domain.ErrPasswordTooWeak,
		},
		{
			name:    "two character classes are not enough",
			policy:  domain.DefaultPasswordPolicy(),
			raw:     "abcdefghijkl",
			wantErr: domain.ErrPasswordTooWeak,
		},
		{
			name:    "lowercase and digits only are not enough",
			policy:  domain.DefaultPasswordPolicy(),
			raw:     "abcdefgh1234",
			wantErr: domain.ErrPasswordTooWeak,
		},
		{
			name:   "three character classes pass",
			policy: domain.DefaultPasswordPolicy(),
			raw:    "abcdefgh123Z",
		},
		{
			name:   "symbols count as a character class",
			policy: domain.DefaultPasswordPolicy(),
			raw:    "Abcdefghij!!",
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.policy.Validate(tc.raw)

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

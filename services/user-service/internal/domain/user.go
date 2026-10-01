package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Limits of the profile fields, counted in characters.
const (
	minFullNameRunes = 2
	maxFullNameRunes = 120

	minPhoneRunes = 10
	maxPhoneRunes = 15
	maxPhoneInput = 24

	maxHashRunes = 512
)

// User is an account of the library: a reader or a librarian.
//
// PasswordHash is exported because the repository is the component that reads
// and writes it; it is the one field the transport layer must never map onto a
// message, which internal/handler keeps as an explicit test case.
type User struct {
	ID           uuid.UUID
	Email        Email
	PasswordHash string
	FullName     string
	Phone        string
	Role         Role
	Status       UserStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time

	// LastLogin is zero while the account never signed in.
	LastLogin time.Time
}

// NewUser validates the raw input and returns a ready to store account. The
// password arrives already hashed: producing the hash is the job of
// internal/security, the domain only refuses an empty or absurd one.
func NewUser(email, passwordHash, fullName, phone string, role Role) (*User, error) {
	mail, err := ParseEmail(email)
	if err != nil {
		return nil, err
	}

	if err := validatePasswordHash(passwordHash); err != nil {
		return nil, err
	}

	name, err := ValidateFullName(fullName)
	if err != nil {
		return nil, err
	}

	number, err := ValidatePhone(phone)
	if err != nil {
		return nil, err
	}

	if !role.Valid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidRole, role)
	}

	return &User{
		ID:           uuid.New(),
		Email:        mail,
		PasswordHash: passwordHash,
		FullName:     name,
		Phone:        number,
		Role:         role,
		Status:       UserStatusActive,
	}, nil
}

// SetPasswordHash replaces the credentials with a fresh encoding.
func (u *User) SetPasswordHash(hash string) error {
	if err := validatePasswordHash(hash); err != nil {
		return err
	}

	u.PasswordHash = hash

	return nil
}

// MarkLogin records the moment of a successful sign in.
func (u *User) MarkLogin(at time.Time) {
	u.LastLogin = at
}

// Deactivate blocks the account. The row stays: the loan history of a reader has
// to survive the account that produced it.
func (u *User) Deactivate() {
	u.Status = UserStatusDeactivated
}

// Restore reopens a blocked account.
func (u *User) Restore() {
	u.Status = UserStatusActive
}

// IsActive reports whether the account may sign in.
func (u *User) IsActive() bool {
	return u.Status == UserStatusActive
}

// UserFilter narrows down the listing of the accounts. Empty Role and Status
// mean "any", which is what the proto enums UNSPECIFIED map onto.
type UserFilter struct {
	// Query is matched against the full name, the email and the phone.
	Query string

	Role   Role
	Status UserStatus

	Limit  int
	Offset int
}

// UserUpdate carries the optional fields of UpdateUser. A nil field means "keep
// the current value", which is exactly what a field mask buys us.
type UserUpdate struct {
	Email    *string
	FullName *string
	Phone    *string
	Role     *Role
	Status   *UserStatus
}

// IsEmpty reports whether the update would change nothing.
func (u UserUpdate) IsEmpty() bool {
	return u.Email == nil && u.FullName == nil && u.Phone == nil && u.Role == nil && u.Status == nil
}

// Apply validates and writes the update. It mutates the user only when every
// present field is valid, so a rejected update leaves the entity untouched.
func (u *User) Apply(update UserUpdate) error {
	next := *u

	if update.Email != nil {
		mail, err := ParseEmail(*update.Email)
		if err != nil {
			return err
		}

		next.Email = mail
	}

	if update.FullName != nil {
		name, err := ValidateFullName(*update.FullName)
		if err != nil {
			return err
		}

		next.FullName = name
	}

	if update.Phone != nil {
		number, err := ValidatePhone(*update.Phone)
		if err != nil {
			return err
		}

		next.Phone = number
	}

	if update.Role != nil {
		if !update.Role.Valid() {
			return fmt.Errorf("%w: %q", ErrInvalidRole, *update.Role)
		}

		next.Role = *update.Role
	}

	if update.Status != nil {
		if !update.Status.Valid() {
			return fmt.Errorf("%w: %q", ErrInvalidStatus, *update.Status)
		}

		next.Status = *update.Status
	}

	*u = next

	return nil
}

// ValidateFullName collapses the whitespace of a display name and checks what is
// left.
func ValidateFullName(raw string) (string, error) {
	name := strings.Join(strings.Fields(raw), " ")

	switch {
	case name == "":
		return "", ErrEmptyFullName
	case countRunes(name) < minFullNameRunes:
		return "", fmt.Errorf("%w: at least %d characters", ErrEmptyFullName, minFullNameRunes)
	case countRunes(name) > maxFullNameRunes:
		return "", fmt.Errorf("%w: at most %d characters", ErrInvalidFullName, maxFullNameRunes)
	}

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == ' ' || r == '-' || r == '\'' || r == '.':
		default:
			return "", fmt.Errorf("%w: unsupported character %q", ErrInvalidFullName, r)
		}
	}

	return name, nil
}

// ValidatePhone normalises a phone number: the formatting characters carry no
// information for the library, what stays has to be a dialable number.
func ValidatePhone(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	if countRunes(trimmed) > maxPhoneInput {
		return "", fmt.Errorf("%w: at most %d characters", ErrInvalidPhone, maxPhoneInput)
	}

	var (
		digits  strings.Builder
		started bool
	)

	for _, r := range trimmed {
		switch {
		case r == '+' && !started:
			digits.WriteRune(r)
			started = true
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
			started = true
		case r == ' ' || r == '-' || r == '(' || r == ')':
			// Formatting characters are dropped on purpose.
		default:
			return "", fmt.Errorf("%w: unsupported character %q", ErrInvalidPhone, r)
		}
	}

	number := digits.String()

	// What is left is "+" and ASCII digits only, so the byte length of the part
	// after the sign is the number of digits.
	count := len(strings.TrimPrefix(number, "+"))

	if count < minPhoneRunes || count > maxPhoneRunes {
		return "", fmt.Errorf("%w: got %d digits, want %d to %d", ErrInvalidPhone, count, minPhoneRunes, maxPhoneRunes)
	}

	return number, nil
}

func validatePasswordHash(hash string) error {
	trimmed := strings.TrimSpace(hash)

	switch {
	case trimmed == "":
		return fmt.Errorf("%w: password hash must not be empty", ErrPasswordTooWeak)
	case countRunes(trimmed) > maxHashRunes:
		return fmt.Errorf("%w: password hash is too long", ErrPasswordTooWeak)
	}

	return nil
}

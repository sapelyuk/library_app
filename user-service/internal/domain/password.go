package domain

import (
	"fmt"
	"strings"
	"unicode"
)

// Bounds of the password policy. MinLength is configurable, the rest of the
// rules are the business rules of the library.
const (
	// AbsoluteMinPasswordLength is what even a relaxed configuration cannot
	// go below.
	AbsoluteMinPasswordLength = 8

	// MaxPasswordLength keeps the argon2 input bounded: memory-hard hashing
	// of an unbounded string is a denial of service vector.
	MaxPasswordLength = 256

	// MinCharacterClasses is how many of the four classes (lower, upper,
	// digit, symbol) a password has to mix.
	MinCharacterClasses = 3
)

// PasswordPolicy describes what counts as an acceptable password.
type PasswordPolicy struct {
	// MinLength is the minimal number of runes.
	MinLength int
}

// DefaultPasswordPolicy is used when the service is configured without an
// explicit length.
func DefaultPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{MinLength: 12}
}

// Validate checks a plaintext password against the policy. It is called before
// hashing, both on registration and on every password change.
func (p PasswordPolicy) Validate(raw string) error {
	minLength := p.MinLength
	if minLength < AbsoluteMinPasswordLength {
		minLength = AbsoluteMinPasswordLength
	}

	password := []rune(raw)

	switch {
	case len(password) < minLength:
		return fmt.Errorf("%w: at least %d characters", ErrPasswordTooShort, minLength)
	case len(password) > MaxPasswordLength:
		return fmt.Errorf("%w: at most %d characters", ErrPasswordTooLong, MaxPasswordLength)
	}

	if strings.ContainsAny(raw, " \t") {
		return fmt.Errorf("%w: spaces and tabs are not allowed", ErrPasswordTooWeak)
	}

	if classes := characterClasses(raw); classes < MinCharacterClasses {
		return fmt.Errorf("%w: got %d of %d", ErrPasswordTooWeak, classes, MinCharacterClasses)
	}

	return nil
}

// characterClasses counts the groups of characters present in the password.
func characterClasses(raw string) int {
	var lower, upper, digit, symbol bool

	for _, r := range raw {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			symbol = true
		}
	}

	count := 0
	for _, present := range []bool{lower, upper, digit, symbol} {
		if present {
			count++
		}
	}

	return count
}

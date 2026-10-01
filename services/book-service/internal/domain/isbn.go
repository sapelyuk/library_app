package domain

import (
	"strings"
)

// ISBN is a normalized (hyphen and space free) book identifier.
type ISBN string

const (
	ISBN10Length = 10
	ISBN13Length = 13
)

// ParseISBN strips separators and validates the checksum of an ISBN-10/ISBN-13.
func ParseISBN(raw string) (ISBN, error) {
	normalized := normalizeISBN(raw)

	switch len(normalized) {
	case ISBN10Length:
		if !validISBN10(normalized) {
			return "", formatInvalidISBN(raw)
		}
	case ISBN13Length:
		if !validISBN13(normalized) {
			return "", formatInvalidISBN(raw)
		}
	default:
		return "", formatInvalidISBN(raw)
	}

	return ISBN(normalized), nil
}

// String implements fmt.Stringer.
func (i ISBN) String() string {
	return string(i)
}

func formatInvalidISBN(raw string) error {
	return ErrInvalidISBN
}

func normalizeISBN(raw string) string {
	var builder strings.Builder
	builder.Grow(len(raw))

	for _, r := range strings.ToUpper(strings.TrimSpace(raw)) {
		switch {
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == 'X':
			builder.WriteRune(r)
		case r == '-' || r == ' ':
			// Separators carry no information for the checksum.
		default:
			// Unknown symbol makes the value invalid, keep it to fail the length check.
			return "\x00"
		}
	}

	return builder.String()
}

func validISBN10(value string) bool {
	checksum := 0

	for i := 0; i < ISBN10Length; i++ {
		char := value[i]

		switch {
		case char >= '0' && char <= '9':
			checksum += int(char-'0') * (ISBN10Length - i)
		case char == 'X' && i == ISBN10Length-1:
			checksum += 10
		default:
			return false
		}
	}

	return checksum%11 == 0
}

func validISBN13(value string) bool {
	checksum := 0

	for i := 0; i < ISBN13Length; i++ {
		char := value[i]
		if char < '0' || char > '9' {
			return false
		}

		weight := 1
		if i%2 == 1 {
			weight = 3
		}
		checksum += int(char-'0') * weight
	}

	return checksum%10 == 0
}

package domain

import (
	"fmt"
	"strings"
)

// Limits of the email grammar the service accepts. They follow RFC 5321:
// 64 characters in the local part, 253 in the domain part.
const (
	maxEmailLength    = 254
	maxLocalLength    = 64
	minDomainLabels   = 2
	minTLDLabelSize   = 2
	maxDomainLabelLen = 63
)

// Email is a normalised address of a user. Normalisation trims the surrounding
// spaces and lower-cases everything: the local part is treated case
// insensitively as well, which matches how the mail systems of the library
// work and keeps the uniqueness constraint meaningful.
type Email string

// ParseEmail normalises and validates a raw address.
func ParseEmail(raw string) (Email, error) {
	normalized := Email(strings.ToLower(strings.TrimSpace(raw)))

	if len(normalized) == 0 {
		return "", fmt.Errorf("%w: email must not be empty", ErrInvalidEmail)
	}

	if len(normalized) > maxEmailLength {
		return "", fmt.Errorf("%w: email is longer than %d characters", ErrInvalidEmail, maxEmailLength)
	}

	local, domain, found := strings.Cut(string(normalized), "@")
	if !found || strings.Contains(domain, "@") {
		return "", fmt.Errorf("%w: email must contain exactly one @", ErrInvalidEmail)
	}

	if err := validateLocalPart(local); err != nil {
		return "", err
	}

	if err := validateDomain(domain); err != nil {
		return "", err
	}

	return normalized, nil
}

// String returns the normalized address.
func (e Email) String() string {
	return string(e)
}

func validateLocalPart(local string) error {
	if local == "" {
		return fmt.Errorf("%w: local part must not be empty", ErrInvalidEmail)
	}

	if len(local) > maxLocalLength {
		return fmt.Errorf("%w: local part is longer than %d characters", ErrInvalidEmail, maxLocalLength)
	}

	if strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") {
		return fmt.Errorf("%w: local part has a misplaced dot", ErrInvalidEmail)
	}

	for _, r := range local {
		if !isEmailLocalChar(r) {
			return fmt.Errorf("%w: local part has unsupported character %q", ErrInvalidEmail, r)
		}
	}

	return nil
}

func validateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("%w: domain must not be empty", ErrInvalidEmail)
	}

	labels := strings.Split(domain, ".")
	if len(labels) < minDomainLabels {
		return fmt.Errorf("%w: domain must have at least %d labels", ErrInvalidEmail, minDomainLabels)
	}

	for i, label := range labels {
		if len(label) == 0 || len(label) > maxDomainLabelLen {
			return fmt.Errorf("%w: domain label %q has invalid length", ErrInvalidEmail, label)
		}

		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return fmt.Errorf("%w: domain label %q must not start or end with a hyphen", ErrInvalidEmail, label)
		}

		for _, r := range label {
			if !isEmailDomainChar(r) {
				return fmt.Errorf("%w: domain label %q has unsupported character %q", ErrInvalidEmail, label, r)
			}
		}

		last := i == len(labels)-1
		if last && (len(label) < minTLDLabelSize || !isAllLetters(label)) {
			return fmt.Errorf("%w: top level domain %q is not alphabetic", ErrInvalidEmail, label)
		}
	}

	return nil
}

func isEmailLocalChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		return true
	case r == '.' || r == '_' || r == '%' || r == '+' || r == '-':
		return true
	default:
		return false
	}
}

func isEmailDomainChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
}

func isAllLetters(value string) bool {
	for _, r := range value {
		if r < 'a' || r > 'z' {
			return false
		}
	}

	return value != ""
}

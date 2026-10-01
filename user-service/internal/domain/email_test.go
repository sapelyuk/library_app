package domain_test

import (
	"errors"
	"strings"
	"testing"

	"library_app/user-service/internal/domain"
)

func TestParseEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    domain.Email
		wantErr bool
	}{
		{name: "plain address", raw: "reader@example.com", want: "reader@example.com"},
		{name: "surrounding spaces are trimmed", raw: "  reader@example.com  ", want: "reader@example.com"},
		{name: "everything is lower cased", raw: "  ReAdEr@ExAmPlE.CoM ", want: "reader@example.com"},
		{name: "dots in the local part", raw: "first.last@example.com", want: "first.last@example.com"},
		{name: "plus tag", raw: "reader+news@example.com", want: "reader+news@example.com"},
		{name: "underscore, percent and hyphen", raw: "r_name%t-x@example.com", want: "r_name%t-x@example.com"},
		{name: "subdomain and two part tld", raw: "reader@mail.example.co.uk", want: "reader@mail.example.co.uk"},
		{name: "single character local part", raw: "a@example.com", want: "a@example.com"},
		{name: "digits only local part", raw: "12345@example.com", want: "12345@example.com"},
		{name: "maximal local part", raw: strings.Repeat("a", 64) + "@example.com", want: domain.Email(strings.Repeat("a", 64) + "@example.com")},

		{name: "empty", raw: "", wantErr: true},
		{name: "spaces only", raw: "   ", wantErr: true},
		{name: "no at sign", raw: "reader.example.com", wantErr: true},
		{name: "two at signs", raw: "read@er@example.com", wantErr: true},
		{name: "empty local part", raw: "@example.com", wantErr: true},
		{name: "empty domain", raw: "reader@", wantErr: true},
		{name: "local part longer than 64", raw: strings.Repeat("a", 65) + "@example.com", wantErr: true},
		{name: "leading dot in local part", raw: ".reader@example.com", wantErr: true},
		{name: "trailing dot in local part", raw: "reader.@example.com", wantErr: true},
		{name: "double dot in local part", raw: "read..er@example.com", wantErr: true},
		{name: "at sign inside local part", raw: "re@der", wantErr: true},
		{name: "bang in local part", raw: "read!er@example.com", wantErr: true},
		{name: "cyrillic in local part", raw: "читатель@example.com", wantErr: true},
		{name: "domain without a dot", raw: "reader@localhost", wantErr: true},
		{name: "domain label starts with a hyphen", raw: "reader@-example.com", wantErr: true},
		{name: "domain label ends with a hyphen", raw: "reader@example-.com", wantErr: true},
		{name: "empty domain label", raw: "reader@example..com", wantErr: true},
		{name: "underscore in domain", raw: "reader@ex_ample.com", wantErr: true},
		{name: "space in domain", raw: "reader@exa mple.com", wantErr: true},
		{name: "one character tld", raw: "reader@example.c", wantErr: true},
		{name: "numeric tld", raw: "reader@example.123", wantErr: true},
		{name: "digit inside tld", raw: "reader@example.c0m", wantErr: true},
		{name: "domain label longer than 63", raw: "reader@" + strings.Repeat("a", 64) + ".com", wantErr: true},
		{name: "address longer than 254", raw: strings.Repeat("a", 64) + "@" + strings.Repeat(strings.Repeat("b", 63)+".", 4) + "com", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.ParseEmail(tc.raw)

			if tc.wantErr {
				if !errors.Is(err, domain.ErrInvalidEmail) {
					t.Fatalf("want ErrInvalidEmail, got %v", err)
				}

				if got != "" {
					t.Fatalf("rejected address must come back empty, got %q", got)
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

func TestEmailString(t *testing.T) {
	t.Parallel()

	mail, err := domain.ParseEmail("Reader@Example.COM")
	if err != nil {
		t.Fatalf("ParseEmail: unexpected error: %v", err)
	}

	if got := mail.String(); got != "reader@example.com" {
		t.Fatalf("String() = %q, want %q", got, "reader@example.com")
	}
}

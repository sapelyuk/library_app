package domain_test

import (
	"errors"
	"testing"

	"github.com/sapelyuk/smart-library/services/book-service/internal/domain"
)

func TestParseISBN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    domain.ISBN
		wantErr bool
	}{
		{name: "isbn13 with hyphens", raw: "978-3-16-148410-0", want: "9783161484100"},
		{name: "isbn13 without hyphens", raw: "9780306406157", want: "9780306406157"},
		{name: "isbn10 with hyphens", raw: "0-306-40615-2", want: "0306406152"},
		{name: "isbn10 with check digit X", raw: "0-9752298-0-x", want: "097522980X"},
		{name: "spaces are ignored", raw: " 978 3 16 148410 0 ", want: "9783161484100"},
		{name: "wrong isbn13 checksum", raw: "978-3-16-148410-1", wantErr: true},
		{name: "wrong isbn10 checksum", raw: "0-306-40615-3", wantErr: true},
		{name: "too short", raw: "12345", wantErr: true},
		{name: "letters", raw: "isbn-978-3-16", wantErr: true},
		{name: "x in the middle", raw: "97831X1484100", wantErr: true},
		{name: "empty", raw: "", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.ParseISBN(tc.raw)

			if tc.wantErr {
				if !errors.Is(err, domain.ErrInvalidISBN) {
					t.Fatalf("want ErrInvalidISBN, got %v", err)
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

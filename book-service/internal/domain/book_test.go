package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"library_app/book-service/internal/domain"
)

// Валидные номера: контрольные цифры посчитаны по алгоритмам ISBN-10/ISBN-13.
const (
	validISBN13 = "978-0-306-40615-7"
	validISBN10 = "0-306-40615-2"
)

func TestNewBook(t *testing.T) {
	t.Parallel()

	book, err := domain.NewBook(domain.NewBookParams{
		Title:         "  The Go Programming Language  ",
		Author:        "  Alan Donovan  ",
		Publisher:     "  Addison-Wesley  ",
		ISBN:          validISBN13,
		PublishedYear: 2015,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if book.Title != "The Go Programming Language" {
		t.Fatalf("title not trimmed: %q", book.Title)
	}

	if book.Author != "Alan Donovan" {
		t.Fatalf("author not trimmed: %q", book.Author)
	}

	if book.Publisher != "Addison-Wesley" {
		t.Fatalf("publisher not trimmed: %q", book.Publisher)
	}

	if book.ISBN != "9780306406157" {
		t.Fatalf("unexpected isbn: %q", book.ISBN)
	}

	if book.ID == uuid.Nil {
		t.Fatal("expected generated id")
	}

	if book.CreatedAt.IsZero() || book.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to be set")
	}
}

func TestNewBookValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		params  domain.NewBookParams
		wantErr error
	}{
		{
			name:    "empty title",
			params:  domain.NewBookParams{Title: "   ", Author: "a", ISBN: validISBN13, PublishedYear: 2015},
			wantErr: domain.ErrEmptyTitle,
		},
		{
			name:    "empty author",
			params:  domain.NewBookParams{Title: "t", Author: "  ", ISBN: validISBN13, PublishedYear: 2015},
			wantErr: domain.ErrEmptyAuthor,
		},
		{
			name:    "bad isbn",
			params:  domain.NewBookParams{Title: "t", Author: "a", ISBN: "000", PublishedYear: 2015},
			wantErr: domain.ErrInvalidISBN,
		},
		{
			name:    "year before printing press",
			params:  domain.NewBookParams{Title: "t", Author: "a", ISBN: validISBN13, PublishedYear: 1400},
			wantErr: domain.ErrInvalidPublishedYear,
		},
		{
			name:    "year far in the future",
			params:  domain.NewBookParams{Title: "t", Author: "a", ISBN: validISBN13, PublishedYear: 2200},
			wantErr: domain.ErrInvalidPublishedYear,
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := domain.NewBook(tc.params); !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestBookApply(t *testing.T) {
	t.Parallel()

	book, err := domain.NewBook(domain.NewBookParams{
		Title:         "Original",
		Author:        "Author",
		Publisher:     "Publisher",
		ISBN:          validISBN13,
		PublishedYear: 2015,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedAt := book.UpdatedAt

	// Гарантируем, что часы точно сделают тик между снимком и Apply,
	// иначе строгая проверка After() нестабильна на быстрых машинах.
	time.Sleep(time.Millisecond)

	// Обновляется только переданное поле, остальные остаются как были.
	if err := book.Apply(domain.BookUpdate{Title: ptr("Updated")}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if book.Title != "Updated" {
		t.Fatalf("title not updated: %q", book.Title)
	}

	if book.Author != "Author" || book.Publisher != "Publisher" || book.PublishedYear != 2015 {
		t.Fatalf("untouched fields changed: %+v", book)
	}

	if book.ISBN != "9780306406157" {
		t.Fatalf("isbn must stay unchanged, got %q", book.ISBN)
	}

	if !book.UpdatedAt.After(updatedAt) {
		t.Fatal("expected UpdatedAt to move forward")
	}

	// ISBN проходит ту же валидацию, что и при создании.
	if err := book.Apply(domain.BookUpdate{ISBN: ptr(validISBN10)}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if book.ISBN != "0306406152" {
		t.Fatalf("isbn not updated, got %q", book.ISBN)
	}

	if err := book.Apply(domain.BookUpdate{ISBN: ptr("bad")}); !errors.Is(err, domain.ErrInvalidISBN) {
		t.Fatalf("want %v, got %v", domain.ErrInvalidISBN, err)
	}

	if err := book.Apply(domain.BookUpdate{Title: ptr("  ")}); !errors.Is(err, domain.ErrEmptyTitle) {
		t.Fatalf("want %v, got %v", domain.ErrEmptyTitle, err)
	}
}

func TestParseBookISBN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    domain.ISBN
		wantErr bool
	}{
		{name: "isbn13 with hyphens", value: validISBN13, want: "9780306406157"},
		{name: "isbn13 without hyphens", value: "9780306406157", want: "9780306406157"},
		{name: "isbn13 wrong checksum", value: "978-0-306-40615-2", wantErr: true},
		{name: "isbn10 with hyphens", value: validISBN10, want: "0306406152"},
		{name: "isbn10 check digit X", value: "0-9752298-0-X", want: "097522980X"},
		{name: "isbn10 wrong checksum", value: "0-306-40615-3", wantErr: true},
		{name: "letters", value: "978-0-306-AAAA-A", wantErr: true},
		{name: "too short", value: "9780306", wantErr: true},
		{name: "empty", value: "", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.ParseISBN(tc.value)

			switch {
			case tc.wantErr:
				if err == nil {
					t.Fatalf("expected error, got isbn %q", got)
				}
				if !errors.Is(err, domain.ErrInvalidISBN) {
					t.Fatalf("want %v, got %v", domain.ErrInvalidISBN, err)
				}
			case err != nil:
				t.Fatalf("unexpected error: %v", err)
			case got != tc.want:
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestNewCopy(t *testing.T) {
	t.Parallel()

	bookID := uuid.New()

	item, err := domain.NewCopy(bookID, "  BC-001  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if item.Barcode != "BC-001" {
		t.Fatalf("barcode not trimmed: %q", item.Barcode)
	}

	if item.Status != domain.CopyStatusAvailable {
		t.Fatalf("new copy must be available, got %q", item.Status)
	}

	if item.BookID != bookID || item.ID == uuid.Nil {
		t.Fatalf("unexpected identity: %+v", item)
	}

	if item.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be set")
	}

	if _, err := domain.NewCopy(bookID, "   "); !errors.Is(err, domain.ErrEmptyBarcode) {
		t.Fatalf("want %v, got %v", domain.ErrEmptyBarcode, err)
	}
}

func TestCopyStatusTransitions(t *testing.T) {
	t.Parallel()

	newCopy := func(t *testing.T) *domain.Copy {
		t.Helper()

		item, err := domain.NewCopy(uuid.New(), "BC-001")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		return item
	}

	t.Run("borrow and return", func(t *testing.T) {
		t.Parallel()

		item := newCopy(t)

		if err := item.MarkOnLoan(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if item.Status != domain.CopyStatusOnLoan {
			t.Fatalf("want %q, got %q", domain.CopyStatusOnLoan, item.Status)
		}

		// Выдать тот же экземпляр повторно нельзя.
		if err := item.MarkOnLoan(); !errors.Is(err, domain.ErrCopyNotAvailable) {
			t.Fatalf("want %v, got %v", domain.ErrCopyNotAvailable, err)
		}

		if err := item.Return(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if item.Status != domain.CopyStatusAvailable {
			t.Fatalf("want %q, got %q", domain.CopyStatusAvailable, item.Status)
		}

		// Возврат невзятого экземпляра — тоже ошибка.
		if err := item.Return(); !errors.Is(err, domain.ErrCopyNotAvailable) {
			t.Fatalf("want %v, got %v", domain.ErrCopyNotAvailable, err)
		}
	})

	t.Run("status validity", func(t *testing.T) {
		t.Parallel()

		for _, status := range []domain.CopyStatus{
			domain.CopyStatusAvailable,
			domain.CopyStatusOnLoan,
			domain.CopyStatusLost,
			domain.CopyStatusMaintenance,
		} {
			if !status.Valid() {
				t.Fatalf("status %q must be valid", status)
			}
		}

		if domain.CopyStatus("BROKEN").Valid() {
			t.Fatal("unknown status must be invalid")
		}
	})
}

func TestPublishedYearBoundary(t *testing.T) {
	t.Parallel()

	// 1445 — первая печатная книга, нижняя граница включительно.
	if _, err := domain.NewBook(domain.NewBookParams{
		Title: "t", Author: "a", ISBN: validISBN13, PublishedYear: 1445,
	}); err != nil {
		t.Fatalf("year 1445 must be accepted: %v", err)
	}

	if _, err := domain.NewBook(domain.NewBookParams{
		Title: "t", Author: "a", ISBN: validISBN13, PublishedYear: 1444,
	}); !errors.Is(err, domain.ErrInvalidPublishedYear) {
		t.Fatalf("want %v, got %v", domain.ErrInvalidPublishedYear, err)
	}

	// Следующий год допустим (предзаказ), ещё через год — уже нет.
	next := int32(time.Now().UTC().Year() + 1)

	if _, err := domain.NewBook(domain.NewBookParams{
		Title: "t", Author: "a", ISBN: validISBN13, PublishedYear: next,
	}); err != nil {
		t.Fatalf("year %d must be accepted: %v", next, err)
	}

	if _, err := domain.NewBook(domain.NewBookParams{
		Title: "t", Author: "a", ISBN: validISBN13, PublishedYear: next + 1,
	}); !errors.Is(err, domain.ErrInvalidPublishedYear) {
		t.Fatalf("want %v, got %v", domain.ErrInvalidPublishedYear, err)
	}
}

func ptr[T any](value T) *T {
	return &value
}

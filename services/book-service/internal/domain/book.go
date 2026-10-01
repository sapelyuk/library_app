package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// minPublishedYear is the year of the first printed book (Gutenberg Bible).
const minPublishedYear = 1445

// Book is a catalog entry describing a single edition.
type Book struct {
	ID            uuid.UUID
	ISBN          ISBN
	Title         string
	Author        string
	Publisher     string
	PublishedYear int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NewBookParams holds raw input of the CreateBook use case.
type NewBookParams struct {
	ISBN          string
	Title         string
	Author        string
	Publisher     string
	PublishedYear int32
}

// BookUpdate holds optional fields, nil keeps the current value.
type BookUpdate struct {
	ISBN          *string
	Title         *string
	Author        *string
	Publisher     *string
	PublishedYear *int32
}

// BookFilter narrows down the catalog listing.
type BookFilter struct {
	// Query is matched against title, author and ISBN.
	Query  string
	Limit  int
	Offset int
}

// NewBook validates the parameters and returns a ready to store book.
func NewBook(params NewBookParams) (*Book, error) {
	isbn, err := ParseISBN(params.ISBN)
	if err != nil {
		return nil, err
	}

	book := &Book{
		ID:            uuid.New(),
		ISBN:          isbn,
		Title:         strings.TrimSpace(params.Title),
		Author:        strings.TrimSpace(params.Author),
		Publisher:     strings.TrimSpace(params.Publisher),
		PublishedYear: int(params.PublishedYear),
	}

	if err := book.validate(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	book.CreatedAt = now
	book.UpdatedAt = now

	return book, nil
}

// Apply validates and applies a partial update in place.
func (b *Book) Apply(update BookUpdate) error {
	next := *b

	if update.ISBN != nil {
		isbn, err := ParseISBN(*update.ISBN)
		if err != nil {
			return err
		}
		next.ISBN = isbn
	}
	if update.Title != nil {
		next.Title = strings.TrimSpace(*update.Title)
	}
	if update.Author != nil {
		next.Author = strings.TrimSpace(*update.Author)
	}
	if update.Publisher != nil {
		next.Publisher = strings.TrimSpace(*update.Publisher)
	}
	if update.PublishedYear != nil {
		next.PublishedYear = int(*update.PublishedYear)
	}

	if err := next.validate(); err != nil {
		return err
	}

	next.UpdatedAt = time.Now().UTC()
	*b = next

	return nil
}

func (b *Book) validate() error {
	if b.Title == "" {
		return ErrEmptyTitle
	}
	if b.Author == "" {
		return ErrEmptyAuthor
	}

	maxYear := time.Now().UTC().Year() + 1
	if b.PublishedYear < minPublishedYear || b.PublishedYear > maxYear {
		return fmt.Errorf("%w: %d, expected %d..%d", ErrInvalidPublishedYear, b.PublishedYear, minPublishedYear, maxYear)
	}

	return nil
}

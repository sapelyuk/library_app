// Package repository defines the storage contracts of the book service.
// Implementations translate storage failures into domain errors so that the
// upper layers never depend on a particular database driver.
package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/sapelyuk/smart-library/services/book-service/internal/domain"
)

// BookRepository stores the catalog of book editions.
type BookRepository interface {
	Create(ctx context.Context, book *domain.Book) error
	Update(ctx context.Context, book *domain.Book) error
	Delete(ctx context.Context, id uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Book, error)
	GetByISBN(ctx context.Context, isbn domain.ISBN) (*domain.Book, error)

	// List returns a page of books and the total number of matching records.
	List(ctx context.Context, filter domain.BookFilter) ([]*domain.Book, int, error)
}

// CopyRepository stores the inventory of physical copies.
type CopyRepository interface {
	CreateCopy(ctx context.Context, copy *domain.Copy) error
	GetCopyByID(ctx context.Context, id uuid.UUID) (*domain.Copy, error)
	ListByBook(ctx context.Context, bookID uuid.UUID) ([]*domain.Copy, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.CopyStatus) error
	Stats(ctx context.Context, bookID uuid.UUID) (domain.CopyStats, error)

	// AcquireAvailable atomically marks the oldest available copy of the book
	// as ON_LOAN and returns it. Concurrent callers never get the same copy.
	// A production implementation uses SELECT ... FOR UPDATE SKIP LOCKED.
	AcquireAvailable(ctx context.Context, bookID uuid.UUID) (*domain.Copy, error)
}

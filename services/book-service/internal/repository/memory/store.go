// Package memory implements the repository contracts inside the process memory.
// It keeps the service runnable and testable without a database; the PostgreSQL
// implementation on top of pgx is meant to be added next to it.
package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/sapelyuk/smart-library/services/book-service/internal/domain"
	"github.com/sapelyuk/smart-library/services/book-service/internal/repository"
)

// Store is a thread safe in-memory implementation of both
// repository.BookRepository and repository.CopyRepository.
type Store struct {
	mu           sync.RWMutex
	books        map[uuid.UUID]*domain.Book
	isbnIndex    map[domain.ISBN]uuid.UUID
	copies       map[uuid.UUID]*domain.Copy
	barcodeIndex map[string]uuid.UUID
}

var (
	_ repository.BookRepository = (*Store)(nil)
	_ repository.CopyRepository = (*Store)(nil)
)

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		books:        make(map[uuid.UUID]*domain.Book),
		isbnIndex:    make(map[domain.ISBN]uuid.UUID),
		copies:       make(map[uuid.UUID]*domain.Copy),
		barcodeIndex: make(map[string]uuid.UUID),
	}
}

// Create stores a new book and reserves its ISBN.
func (s *Store) Create(_ context.Context, book *domain.Book) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id, ok := s.isbnIndex[book.ISBN]; ok && id != book.ID {
		return fmt.Errorf("%w: %s", domain.ErrISBNAlreadyExists, book.ISBN)
	}

	s.books[book.ID] = cloneBook(book)
	s.isbnIndex[book.ISBN] = book.ID

	return nil
}

// Update replaces the stored book, ISBN changes must stay unique.
func (s *Store) Update(_ context.Context, book *domain.Book) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.books[book.ID]
	if !ok {
		return bookNotFound(book.ID)
	}

	if id, ok := s.isbnIndex[book.ISBN]; ok && id != book.ID {
		return fmt.Errorf("%w: %s", domain.ErrISBNAlreadyExists, book.ISBN)
	}

	delete(s.isbnIndex, stored.ISBN)
	s.books[book.ID] = cloneBook(book)
	s.isbnIndex[book.ISBN] = book.ID

	return nil
}

// Delete removes the book together with all of its copies.
func (s *Store) Delete(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.books[id]
	if !ok {
		return bookNotFound(id)
	}

	for copyID, copy := range s.copies {
		if copy.BookID == id {
			delete(s.copies, copyID)
			delete(s.barcodeIndex, copy.Barcode)
		}
	}

	delete(s.isbnIndex, stored.ISBN)
	delete(s.books, id)

	return nil
}

// GetByID returns the book by its identifier.
func (s *Store) GetByID(_ context.Context, id uuid.UUID) (*domain.Book, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	book, ok := s.books[id]
	if !ok {
		return nil, bookNotFound(id)
	}

	return cloneBook(book), nil
}

// GetByISBN returns the book by its normalized ISBN.
func (s *Store) GetByISBN(_ context.Context, isbn domain.ISBN) (*domain.Book, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.isbnIndex[isbn]
	if !ok {
		return nil, fmt.Errorf("%w: isbn %s", domain.ErrNotFound, isbn)
	}

	return cloneBook(s.books[id]), nil
}

// List returns a deterministic page of books and the total match count.
func (s *Store) List(_ context.Context, filter domain.BookFilter) ([]*domain.Book, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := strings.ToLower(strings.TrimSpace(filter.Query))
	matched := make([]*domain.Book, 0, len(s.books))

	for _, book := range s.books {
		if query != "" && !matches(book, query) {
			continue
		}

		matched = append(matched, cloneBook(book))
	}

	sort.Slice(matched, func(i, j int) bool {
		if matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].ID.String() < matched[j].ID.String()
		}

		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	total := len(matched)

	if filter.Offset > 0 {
		if filter.Offset >= total {
			return []*domain.Book{}, total, nil
		}

		matched = matched[filter.Offset:]
	}

	if filter.Limit > 0 && len(matched) > filter.Limit {
		matched = matched[:filter.Limit]
	}

	return matched, total, nil
}

// Create stores a new copy of the book.
func (s *Store) CreateCopy(_ context.Context, copy *domain.Copy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.books[copy.BookID]; !ok {
		return bookNotFound(copy.BookID)
	}

	if id, ok := s.barcodeIndex[copy.Barcode]; ok && id != copy.ID {
		return fmt.Errorf("%w: %s", domain.ErrBarcodeAlreadyExists, copy.Barcode)
	}

	s.copies[copy.ID] = cloneCopy(copy)
	s.barcodeIndex[copy.Barcode] = copy.ID

	return nil
}

// GetByID returns the copy by its identifier.
func (s *Store) GetCopyByID(_ context.Context, id uuid.UUID) (*domain.Copy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copy, ok := s.copies[id]
	if !ok {
		return nil, copyNotFound(id)
	}

	return cloneCopy(copy), nil
}

// ListByBook returns copies of the book ordered by registration time.
func (s *Store) ListByBook(_ context.Context, bookID uuid.UUID) ([]*domain.Copy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.books[bookID]; !ok {
		return nil, bookNotFound(bookID)
	}

	return s.copiesOf(bookID), nil
}

// UpdateStatus changes the lifecycle state of the copy.
func (s *Store) UpdateStatus(_ context.Context, id uuid.UUID, status domain.CopyStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.copies[id]
	if !ok {
		return copyNotFound(id)
	}

	if !status.Valid() {
		return fmt.Errorf("%w: %s", domain.ErrInvalidCopyStatus, status)
	}

	updated := cloneCopy(stored)
	updated.Status = status
	s.copies[id] = updated

	return nil
}

// Stats aggregates the inventory of the book.
func (s *Store) Stats(_ context.Context, bookID uuid.UUID) (domain.CopyStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.books[bookID]; !ok {
		return domain.CopyStats{}, bookNotFound(bookID)
	}

	var stats domain.CopyStats

	for _, copy := range s.copies {
		if copy.BookID != bookID {
			continue
		}

		stats.Total++

		switch copy.Status {
		case domain.CopyStatusAvailable:
			stats.Available++
		case domain.CopyStatusOnLoan:
			stats.OnLoan++
		case domain.CopyStatusLost, domain.CopyStatusMaintenance:
			// Not lendable, counted only in Total.
		}
	}

	return stats, nil
}

// AcquireAvailable marks the oldest available copy as ON_LOAN under a single
// write lock, so parallel borrowers never receive the same copy.
func (s *Store) AcquireAvailable(_ context.Context, bookID uuid.UUID) (*domain.Copy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.books[bookID]; !ok {
		return nil, bookNotFound(bookID)
	}

	available := s.copiesOf(bookID)

	for _, copy := range available {
		if copy.Status != domain.CopyStatusAvailable {
			continue
		}

		if err := copy.MarkOnLoan(); err != nil {
			return nil, err
		}

		s.copies[copy.ID] = cloneCopy(copy)

		return copy, nil
	}

	return nil, fmt.Errorf("%w: book %s", domain.ErrNoAvailableCopies, bookID)
}

// copiesOf returns clones of the book copies ordered by creation time.
// Callers must hold at least the read lock.
func (s *Store) copiesOf(bookID uuid.UUID) []*domain.Copy {
	result := make([]*domain.Copy, 0, len(s.copies))

	for _, copy := range s.copies {
		if copy.BookID == bookID {
			result = append(result, cloneCopy(copy))
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID.String() < result[j].ID.String()
		}

		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result
}

func matches(book *domain.Book, query string) bool {
	return strings.Contains(strings.ToLower(book.Title), query) ||
		strings.Contains(strings.ToLower(book.Author), query) ||
		strings.Contains(strings.ToLower(book.ISBN.String()), query)
}

func cloneBook(book *domain.Book) *domain.Book {
	clone := *book

	return &clone
}

func cloneCopy(copy *domain.Copy) *domain.Copy {
	clone := *copy

	return &clone
}

func bookNotFound(id uuid.UUID) error {
	return fmt.Errorf("%w: %s", domain.ErrNotFound, id)
}

func copyNotFound(id uuid.UUID) error {
	return fmt.Errorf("%w: %s", domain.ErrCopyNotFound, id)
}

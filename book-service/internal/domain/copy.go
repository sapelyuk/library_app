package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CopyStatus is the lifecycle state of a physical copy.
type CopyStatus string

const (
	CopyStatusAvailable   CopyStatus = "AVAILABLE"
	CopyStatusOnLoan      CopyStatus = "ON_LOAN"
	CopyStatusLost        CopyStatus = "LOST"
	CopyStatusMaintenance CopyStatus = "MAINTENANCE"
)

// Valid reports whether the status is known.
func (s CopyStatus) Valid() bool {
	switch s {
	case CopyStatusAvailable, CopyStatusOnLoan, CopyStatusLost, CopyStatusMaintenance:
		return true
	default:
		return false
	}
}

// Copy is a single physical item of a book edition.
type Copy struct {
	ID        uuid.UUID
	BookID    uuid.UUID
	Barcode   string
	Status    CopyStatus
	CreatedAt time.Time
}

// CopyStats aggregates the inventory of one book.
type CopyStats struct {
	Total     int
	Available int
	OnLoan    int
}

// NewCopy registers a new copy in the available state.
func NewCopy(bookID uuid.UUID, barcode string) (*Copy, error) {
	trimmed := strings.TrimSpace(barcode)
	if trimmed == "" {
		return nil, ErrEmptyBarcode
	}

	return &Copy{
		ID:        uuid.New(),
		BookID:    bookID,
		Barcode:   trimmed,
		Status:    CopyStatusAvailable,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// MarkOnLoan hands the copy out, only an available copy can be borrowed.
func (c *Copy) MarkOnLoan() error {
	if c.Status != CopyStatusAvailable {
		return fmt.Errorf("%w: copy %s is %s", ErrCopyNotAvailable, c.ID, c.Status)
	}

	c.Status = CopyStatusOnLoan

	return nil
}

// Return puts a borrowed copy back into the catalog.
func (c *Copy) Return() error {
	if c.Status != CopyStatusOnLoan {
		return fmt.Errorf("%w: copy %s is %s", ErrCopyNotAvailable, c.ID, c.Status)
	}

	c.Status = CopyStatusAvailable

	return nil
}

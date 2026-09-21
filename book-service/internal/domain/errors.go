package domain

import "errors"

// Domain errors of the book service. Wrap them with fmt.Errorf("%w") on the way
// up and match with errors.Is in the transport layer.
var (
	ErrNotFound             = errors.New("book not found")
	ErrISBNAlreadyExists    = errors.New("book with this ISBN already exists")
	ErrInvalidISBN          = errors.New("invalid ISBN")
	ErrEmptyTitle           = errors.New("title must not be empty")
	ErrEmptyAuthor          = errors.New("author must not be empty")
	ErrInvalidPublishedYear = errors.New("published year out of range")

	ErrCopyNotFound         = errors.New("copy not found")
	ErrEmptyBarcode         = errors.New("barcode must not be empty")
	ErrBarcodeAlreadyExists = errors.New("copy with this barcode already exists")
	ErrInvalidCopyStatus    = errors.New("invalid copy status")
	ErrCopyNotAvailable     = errors.New("copy is not available")
	ErrNoAvailableCopies    = errors.New("book has no available copies")
	ErrBookHasActiveLoans   = errors.New("book has copies on loan")
)

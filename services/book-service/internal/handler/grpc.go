// Package handler adapts the gRPC transport to the service layer: it parses
// requests, maps domain errors onto gRPC status codes and converts entities
// back to protobuf messages.
package handler

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	bookv1 "github.com/sapelyuk/smart-library/services/book-service/gen/go/book/v1"
	"github.com/sapelyuk/smart-library/services/book-service/internal/domain"
	"github.com/sapelyuk/smart-library/services/book-service/internal/service"
)

// GRPCServer implements bookv1.BookServiceServer.
type GRPCServer struct {
	bookv1.UnimplementedBookServiceServer

	service *service.BookService
	log     *slog.Logger
}

var _ bookv1.BookServiceServer = (*GRPCServer)(nil)

// NewGRPCServer builds the gRPC adapter of the book service.
func NewGRPCServer(svc *service.BookService, log *slog.Logger) *GRPCServer {
	return &GRPCServer{service: svc, log: log}
}

// CreateBook adds an edition to the catalog.
func (s *GRPCServer) CreateBook(ctx context.Context, req *bookv1.CreateBookRequest) (*bookv1.Book, error) {
	book, err := s.service.CreateBook(ctx, service.CreateBookInput{
		ISBN:          req.GetIsbn(),
		Title:         req.GetTitle(),
		Author:        req.GetAuthor(),
		Publisher:     req.GetPublisher(),
		PublishedYear: req.GetPublishedYear(),
	})
	if err != nil {
		return nil, s.mapError("CreateBook", err)
	}

	// A brand new book has no copies registered yet.
	return toProtoBook(book, domain.CopyStats{}), nil
}

// GetBook returns one catalog entry with copy statistics.
func (s *GRPCServer) GetBook(ctx context.Context, req *bookv1.GetBookRequest) (*bookv1.Book, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}

	view, err := s.service.GetBook(ctx, id)
	if err != nil {
		return nil, s.mapError("GetBook", err)
	}

	return toProtoBook(view.Book, view.Stats), nil
}

// ListBooks returns a page of the catalog.
func (s *GRPCServer) ListBooks(ctx context.Context, req *bookv1.ListBooksRequest) (*bookv1.ListBooksResponse, error) {
	views, total, err := s.service.ListBooks(ctx, domain.BookFilter{
		Query:  req.GetQuery(),
		Limit:  int(req.GetLimit()),
		Offset: int(req.GetOffset()),
	})
	if err != nil {
		return nil, s.mapError("ListBooks", err)
	}

	books := make([]*bookv1.Book, 0, len(views))

	for _, view := range views {
		books = append(books, toProtoBook(view.Book, view.Stats))
	}

	return &bookv1.ListBooksResponse{Books: books, Total: int32(total)}, nil
}

// UpdateBook applies a partial update of the catalog entry.
func (s *GRPCServer) UpdateBook(ctx context.Context, req *bookv1.UpdateBookRequest) (*bookv1.Book, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}

	view, err := s.service.UpdateBook(ctx, id, domain.BookUpdate{
		ISBN:          req.Isbn,
		Title:         req.Title,
		Author:        req.Author,
		Publisher:     req.Publisher,
		PublishedYear: req.PublishedYear,
	})
	if err != nil {
		return nil, s.mapError("UpdateBook", err)
	}

	return toProtoBook(view.Book, view.Stats), nil
}

// DeleteBook removes a book that has no copies on loan.
func (s *GRPCServer) DeleteBook(ctx context.Context, req *bookv1.DeleteBookRequest) (*emptypb.Empty, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}

	if err := s.service.DeleteBook(ctx, id); err != nil {
		return nil, s.mapError("DeleteBook", err)
	}

	return &emptypb.Empty{}, nil
}

// AddBookCopy registers a physical copy of the book.
func (s *GRPCServer) AddBookCopy(ctx context.Context, req *bookv1.AddBookCopyRequest) (*bookv1.BookCopy, error) {
	id, err := parseUUID(req.GetBookId())
	if err != nil {
		return nil, err
	}

	created, err := s.service.AddBookCopy(ctx, id, req.GetBarcode())
	if err != nil {
		return nil, s.mapError("AddBookCopy", err)
	}

	return toProtoCopy(created), nil
}

// ListBookCopies returns the inventory of the book.
func (s *GRPCServer) ListBookCopies(ctx context.Context, req *bookv1.ListBookCopiesRequest) (*bookv1.ListBookCopiesResponse, error) {
	id, err := parseUUID(req.GetBookId())
	if err != nil {
		return nil, err
	}

	copies, err := s.service.ListBookCopies(ctx, id)
	if err != nil {
		return nil, s.mapError("ListBookCopies", err)
	}

	protoCopies := make([]*bookv1.BookCopy, 0, len(copies))

	for _, item := range copies {
		protoCopies = append(protoCopies, toProtoCopy(item))
	}

	return &bookv1.ListBookCopiesResponse{Copies: protoCopies}, nil
}

// BorrowBookCopy marks the first available copy as ON_LOAN.
func (s *GRPCServer) BorrowBookCopy(ctx context.Context, req *bookv1.BorrowBookCopyRequest) (*bookv1.BookCopy, error) {
	id, err := parseUUID(req.GetBookId())
	if err != nil {
		return nil, err
	}

	item, err := s.service.BorrowBookCopy(ctx, id)
	if err != nil {
		return nil, s.mapError("BorrowBookCopy", err)
	}

	return toProtoCopy(item), nil
}

// ReturnBookCopy marks the copy as AVAILABLE again.
func (s *GRPCServer) ReturnBookCopy(ctx context.Context, req *bookv1.ReturnBookCopyRequest) (*bookv1.BookCopy, error) {
	id, err := parseUUID(req.GetCopyId())
	if err != nil {
		return nil, err
	}

	item, err := s.service.ReturnBookCopy(ctx, id)
	if err != nil {
		return nil, s.mapError("ReturnBookCopy", err)
	}

	return toProtoCopy(item), nil
}

// error maps a domain error onto a gRPC status, unexpected failures are logged
// and reported as codes.Internal without leaking internals to the client.
func (s *GRPCServer) mapError(method string, err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrCopyNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrISBNAlreadyExists), errors.Is(err, domain.ErrBarcodeAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrInvalidISBN),
		errors.Is(err, domain.ErrEmptyTitle),
		errors.Is(err, domain.ErrEmptyAuthor),
		errors.Is(err, domain.ErrInvalidPublishedYear),
		errors.Is(err, domain.ErrEmptyBarcode),
		errors.Is(err, domain.ErrInvalidCopyStatus):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrNoAvailableCopies),
		errors.Is(err, domain.ErrCopyNotAvailable),
		errors.Is(err, domain.ErrBookHasActiveLoans):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, context.Canceled):
		return status.FromContextError(err).Err()
	default:
		s.log.Error("unexpected failure", "method", method, "error", err)

		return status.Error(codes.Internal, "internal error")
	}
}

func parseUUID(raw string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "malformed id: "+err.Error())
	}

	return parsed, nil
}

func toProtoBook(book *domain.Book, stats domain.CopyStats) *bookv1.Book {
	return &bookv1.Book{
		Id:              book.ID.String(),
		Isbn:            book.ISBN.String(),
		Title:           book.Title,
		Author:          book.Author,
		Publisher:       book.Publisher,
		PublishedYear:   int32(book.PublishedYear),
		TotalCopies:     int32(stats.Total),
		AvailableCopies: int32(stats.Available),
		CreatedAt:       timestamppb.New(book.CreatedAt),
		UpdatedAt:       timestamppb.New(book.UpdatedAt),
	}
}

func toProtoCopy(item *domain.Copy) *bookv1.BookCopy {
	return &bookv1.BookCopy{
		Id:        item.ID.String(),
		BookId:    item.BookID.String(),
		Barcode:   item.Barcode,
		Status:    toProtoStatus(item.Status),
		CreatedAt: timestamppb.New(item.CreatedAt),
	}
}

func toProtoStatus(state domain.CopyStatus) bookv1.CopyStatus {
	switch state {
	case domain.CopyStatusAvailable:
		return bookv1.CopyStatus_COPY_STATUS_AVAILABLE
	case domain.CopyStatusOnLoan:
		return bookv1.CopyStatus_COPY_STATUS_ON_LOAN
	case domain.CopyStatusLost:
		return bookv1.CopyStatus_COPY_STATUS_LOST
	case domain.CopyStatusMaintenance:
		return bookv1.CopyStatus_COPY_STATUS_MAINTENANCE
	default:
		return bookv1.CopyStatus_COPY_STATUS_UNSPECIFIED
	}
}

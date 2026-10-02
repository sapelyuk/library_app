package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	bookv1 "github.com/sapelyuk/smart-library/services/book-service/gen/go/book/v1"
	"github.com/sapelyuk/smart-library/services/book-service/internal/handler"
)

// stubClient records the request the gateway built and answers with canned
// values; every method is implemented so the gateway never hits a nil stub.
type stubClient struct {
	bookv1.BookServiceClient

	createIn *bookv1.CreateBookRequest
	getIn    *bookv1.GetBookRequest
	listIn   *bookv1.ListBooksRequest
	updateIn *bookv1.UpdateBookRequest
	deleteIn *bookv1.DeleteBookRequest
	addCopy  *bookv1.AddBookCopyRequest
	listCopy *bookv1.ListBookCopiesRequest
	borrowIn *bookv1.BorrowBookCopyRequest
	returnIn *bookv1.ReturnBookCopyRequest

	// err, when set, is returned by the read methods to exercise the
	// gRPC-to-HTTP status mapping.
	err error
}

const (
	bookID = "11111111-1111-1111-1111-111111111111"
	copyID = "22222222-2222-2222-2222-222222222222"
)

// isValidUUID checks that raw looks like a canonical UUID.
func isValidUUID(raw string) bool {
	return len(raw) == 36 && raw[8] == '-' && raw[13] == '-' && raw[18] == '-' && raw[23] == '-'
}

func cannedBook() *bookv1.Book {
	return &bookv1.Book{
		Id:              bookID,
		Isbn:            "9780134190440",
		Title:           "The Go Programming Language",
		Author:          "Donovan",
		Publisher:       "Addison-Wesley",
		PublishedYear:   2015,
		TotalCopies:     3,
		AvailableCopies: 2,
	}
}

func cannedCopy(state bookv1.CopyStatus) *bookv1.BookCopy {
	return &bookv1.BookCopy{
		Id:      copyID,
		BookId:  bookID,
		Barcode: "BC-000001",
		Status:  state,
	}
}

func (s *stubClient) CreateBook(_ context.Context, in *bookv1.CreateBookRequest, _ ...grpc.CallOption) (*bookv1.Book, error) {
	s.createIn = in

	if s.err != nil {
		return nil, s.err
	}

	book := cannedBook()
	book.Title = in.GetTitle()

	return book, nil
}

func (s *stubClient) GetBook(_ context.Context, in *bookv1.GetBookRequest, _ ...grpc.CallOption) (*bookv1.Book, error) {
	s.getIn = in

	if s.err != nil {
		return nil, s.err
	}

	if !isValidUUID(in.GetId()) {
		return nil, status.Error(codes.InvalidArgument, "invalid book id")
	}

	return cannedBook(), nil
}

func (s *stubClient) ListBooks(_ context.Context, in *bookv1.ListBooksRequest, _ ...grpc.CallOption) (*bookv1.ListBooksResponse, error) {
	s.listIn = in

	if s.err != nil {
		return nil, s.err
	}

	return &bookv1.ListBooksResponse{Books: []*bookv1.Book{cannedBook()}, Total: 1}, nil
}

func (s *stubClient) UpdateBook(_ context.Context, in *bookv1.UpdateBookRequest, _ ...grpc.CallOption) (*bookv1.Book, error) {
	s.updateIn = in

	if s.err != nil {
		return nil, s.err
	}

	book := cannedBook()
	if in.Title != nil {
		book.Title = *in.Title
	}

	return book, nil
}

func (s *stubClient) DeleteBook(_ context.Context, in *bookv1.DeleteBookRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	s.deleteIn = in

	if s.err != nil {
		return nil, s.err
	}

	return &emptypb.Empty{}, nil
}

func (s *stubClient) AddBookCopy(_ context.Context, in *bookv1.AddBookCopyRequest, _ ...grpc.CallOption) (*bookv1.BookCopy, error) {
	s.addCopy = in

	if s.err != nil {
		return nil, s.err
	}

	item := cannedCopy(bookv1.CopyStatus_COPY_STATUS_AVAILABLE)
	item.Barcode = in.GetBarcode()

	return item, nil
}

func (s *stubClient) ListBookCopies(_ context.Context, in *bookv1.ListBookCopiesRequest, _ ...grpc.CallOption) (*bookv1.ListBookCopiesResponse, error) {
	s.listCopy = in

	if s.err != nil {
		return nil, s.err
	}

	return &bookv1.ListBookCopiesResponse{Copies: []*bookv1.BookCopy{
		cannedCopy(bookv1.CopyStatus_COPY_STATUS_AVAILABLE),
	}}, nil
}

func (s *stubClient) BorrowBookCopy(_ context.Context, in *bookv1.BorrowBookCopyRequest, _ ...grpc.CallOption) (*bookv1.BookCopy, error) {
	s.borrowIn = in

	if s.err != nil {
		return nil, s.err
	}

	return cannedCopy(bookv1.CopyStatus_COPY_STATUS_ON_LOAN), nil
}

func (s *stubClient) ReturnBookCopy(_ context.Context, in *bookv1.ReturnBookCopyRequest, _ ...grpc.CallOption) (*bookv1.BookCopy, error) {
	s.returnIn = in

	if s.err != nil {
		return nil, s.err
	}

	return cannedCopy(bookv1.CopyStatus_COPY_STATUS_AVAILABLE), nil
}

func newServer(t *testing.T, client bookv1.BookServiceClient) *httptest.Server {
	t.Helper()

	httpHandler, err := handler.NewREST(client)
	if err != nil {
		t.Fatalf("build rest handler: %v", err)
	}

	server := httptest.NewServer(httpHandler)
	t.Cleanup(server.Close)

	return server
}

// doRequest issues an HTTP call with an optional JSON body and returns the
// response; the body is closed when the test finishes.
func doRequest(t *testing.T, method, url, body string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}

	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}

	t.Cleanup(func() { resp.Body.Close() })

	return resp
}

func decode(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	return decoded
}

func TestCreateBookRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	body := `{"isbn":"978-0-13-419044-0","title":"The Go Programming Language","author":"Donovan","publisher":"Addison-Wesley","published_year":2015}`

	resp := doRequest(t, http.MethodPost, server.URL+"/v1/books", body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	decoded := decode(t, resp)

	if got := decoded["id"]; got != bookID {
		t.Errorf("id = %v, want the id returned by the service", got)
	}

	// Zero values must stay in the payload so the browser sees every counter.
	if _, ok := decoded["totalCopies"]; !ok {
		t.Error("totalCopies is missing from the response, zero values are not emitted")
	}

	if client.createIn.GetPublishedYear() != 2015 {
		t.Errorf("published_year = %d, want 2015: the body was not mapped onto the request", client.createIn.GetPublishedYear())
	}
}

func TestGetBookRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	resp := doRequest(t, http.MethodGet, server.URL+"/v1/books/"+bookID, "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// The path segment must land in the request message.
	if client.getIn.GetId() != bookID {
		t.Errorf("id = %q, want the id from the path", client.getIn.GetId())
	}

	decoded := decode(t, resp)

	if got := decoded["title"]; got != "The Go Programming Language" {
		t.Errorf("title = %v, want the value returned by the service", got)
	}

	// Copy statistics are projected onto the response under camelCase names.
	if got := decoded["availableCopies"]; got != float64(2) {
		t.Errorf("availableCopies = %v, want 2", got)
	}
}

func TestListBooksQueryParameters(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	resp := doRequest(t, http.MethodGet, server.URL+"/v1/books?query=go&limit=5&offset=10", "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if client.listIn.GetQuery() != "go" {
		t.Errorf("query = %q, want %q: the query string was not mapped", client.listIn.GetQuery(), "go")
	}

	if client.listIn.GetLimit() != 5 || client.listIn.GetOffset() != 10 {
		t.Errorf("limit/offset = %d/%d, want 5/10", client.listIn.GetLimit(), client.listIn.GetOffset())
	}

	decoded := decode(t, resp)

	if got := decoded["total"]; got != float64(1) {
		t.Errorf("total = %v, want 1", got)
	}

	books, ok := decoded["books"].([]any)
	if !ok || len(books) != 1 {
		t.Fatalf("books = %v, want a single-element array", decoded["books"])
	}
}

func TestUpdateBookRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	resp := doRequest(t, http.MethodPatch, server.URL+"/v1/books/"+bookID, `{"title":"Second Edition"}`)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if client.updateIn.GetId() != bookID {
		t.Errorf("id = %q, want the id from the path", client.updateIn.GetId())
	}

	// optional field: the presence of the key must reach the service as a set
	// pointer, distinguishable from an absent one.
	if client.updateIn.Title == nil {
		t.Error("title is nil: the optional field was lost in the mapping")
	} else if *client.updateIn.Title != "Second Edition" {
		t.Errorf("title = %q, want %q", *client.updateIn.Title, "Second Edition")
	}

	if client.updateIn.PublishedYear != nil {
		t.Errorf("published_year must stay absent, got %d", *client.updateIn.PublishedYear)
	}
}

func TestDeleteBookRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	resp := doRequest(t, http.MethodDelete, server.URL+"/v1/books/"+bookID, "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if client.deleteIn.GetId() != bookID {
		t.Errorf("id = %q, want the id from the path", client.deleteIn.GetId())
	}

	// Empty maps to an empty JSON object, not to a null or a missing body.
	decoded := decode(t, resp)

	if len(decoded) != 0 {
		t.Errorf("body = %v, want an empty object", decoded)
	}
}

func TestAddBookCopyRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	resp := doRequest(t, http.MethodPost, server.URL+"/v1/books/"+bookID+"/copies", `{"barcode":"BC-000042"}`)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if client.addCopy.GetBookId() != bookID {
		t.Errorf("book_id = %q, want the id from the path", client.addCopy.GetBookId())
	}

	if client.addCopy.GetBarcode() != "BC-000042" {
		t.Errorf("barcode = %q, want it mapped from the body", client.addCopy.GetBarcode())
	}

	decoded := decode(t, resp)

	// The enum travels as its symbolic name so the Swagger UI stays readable.
	if got := decoded["status"]; got != "COPY_STATUS_AVAILABLE" {
		t.Errorf("status = %v, want COPY_STATUS_AVAILABLE", got)
	}
}

func TestListBookCopiesRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	resp := doRequest(t, http.MethodGet, server.URL+"/v1/books/"+bookID+"/copies", "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if client.listCopy.GetBookId() != bookID {
		t.Errorf("book_id = %q, want the id from the path", client.listCopy.GetBookId())
	}

	decoded := decode(t, resp)

	copies, ok := decoded["copies"].([]any)
	if !ok || len(copies) != 1 {
		t.Fatalf("copies = %v, want a single-element array", decoded["copies"])
	}
}

func TestBorrowBookCopyRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	// The book id travels in the URL, the request has no body.
	resp := doRequest(t, http.MethodPost, server.URL+"/v1/books/"+bookID+"/borrow", "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if client.borrowIn.GetBookId() != bookID {
		t.Errorf("book_id = %q, want the id from the path", client.borrowIn.GetBookId())
	}

	decoded := decode(t, resp)

	if got := decoded["status"]; got != "COPY_STATUS_ON_LOAN" {
		t.Errorf("status = %v, want COPY_STATUS_ON_LOAN after a borrow", got)
	}
}

func TestReturnBookCopyRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	// The copy id travels in the URL, the request has no body.
	resp := doRequest(t, http.MethodPost, server.URL+"/v1/copies/"+copyID+"/return", "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if client.returnIn.GetCopyId() != copyID {
		t.Errorf("copy_id = %q, want the id from the path", client.returnIn.GetCopyId())
	}

	decoded := decode(t, resp)

	if got := decoded["status"]; got != "COPY_STATUS_AVAILABLE" {
		t.Errorf("status = %v, want COPY_STATUS_AVAILABLE after a return", got)
	}
}

func TestHTTPStatusMapping(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{
			name:     "not found maps to 404",
			err:      status.Error(codes.NotFound, "book not found"),
			wantCode: http.StatusNotFound,
		},
		{
			name:     "already exists maps to 409",
			err:      status.Error(codes.AlreadyExists, "book with this ISBN already exists"),
			wantCode: http.StatusConflict,
		},
		{
			name:     "invalid argument maps to 400",
			err:      status.Error(codes.InvalidArgument, "invalid ISBN"),
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "failed precondition maps to 400",
			err:      status.Error(codes.FailedPrecondition, "book has no available copies"),
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "internal maps to 500 without leaking details",
			err:      status.Error(codes.Internal, "internal error"),
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &stubClient{err: tc.err}
			server := newServer(t, client)

			resp := doRequest(t, http.MethodGet, server.URL+"/v1/books/"+bookID, "")

			if resp.StatusCode != tc.wantCode {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantCode)
			}

			decoded := decode(t, resp)

			// The gateway reports the proto code number so the client can branch
			// on it, not only on the HTTP status.
			if got := decoded["code"]; got != float64(status.Code(tc.err)) {
				t.Errorf("error code = %v, want %d", got, status.Code(tc.err))
			}
		})
	}
}

func TestMalformedPathIDMapsToBadRequest(t *testing.T) {
	server := newServer(t, &stubClient{})

	// A non-UUID path segment makes the gRPC handler answer INVALID_ARGUMENT,
	// which the gateway translates into HTTP 400.
	resp := doRequest(t, http.MethodGet, server.URL+"/v1/books/not-a-uuid", "")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestUnknownRouteMapsToNotFound(t *testing.T) {
	server := newServer(t, &stubClient{})

	resp := doRequest(t, http.MethodGet, server.URL+"/v1/unknown", "")

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestSwaggerEndpoints(t *testing.T) {
	server := newServer(t, &stubClient{})

	specResp, err := http.Get(server.URL + "/swagger/swagger.json")
	if err != nil {
		t.Fatalf("get swagger.json: %v", err)
	}

	defer specResp.Body.Close()

	if specResp.StatusCode != http.StatusOK {
		t.Fatalf("swagger.json status = %d, want %d", specResp.StatusCode, http.StatusOK)
	}

	if got := specResp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("swagger.json content type = %q, want application/json", got)
	}

	var spec map[string]any
	if err := json.NewDecoder(specResp.Body).Decode(&spec); err != nil {
		t.Fatalf("decode swagger.json: %v", err)
	}

	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("swagger.json has no paths section")
	}

	for _, path := range []string{"/v1/books", "/v1/books/{id}", "/v1/books/{bookId}/borrow", "/v1/copies/{copyId}/return"} {
		if _, ok := paths[path]; !ok {
			t.Errorf("swagger.json is missing path %s", path)
		}
	}

	uiResp, err := http.Get(server.URL + "/swagger/")
	if err != nil {
		t.Fatalf("get /swagger/: %v", err)
	}

	defer uiResp.Body.Close()

	if uiResp.StatusCode != http.StatusOK {
		t.Fatalf("/swagger/ status = %d, want %d", uiResp.StatusCode, http.StatusOK)
	}

	if got := uiResp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("/swagger/ content type = %q, want text/html", got)
	}

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	rootResp, err := noRedirect.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}

	defer rootResp.Body.Close()

	if rootResp.StatusCode != http.StatusFound || rootResp.Header.Get("Location") != "/swagger/" {
		t.Errorf("/ = %d %q, want 302 -> /swagger/", rootResp.StatusCode, rootResp.Header.Get("Location"))
	}
}
